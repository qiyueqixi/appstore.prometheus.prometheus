param(
    [string]$Version = '3.13.0',
    [string]$PackVersion = '3.13.0-16',
    [ValidateSet('amd64', 'arm64')]
    [string[]]$Architectures = @('amd64', 'arm64'),
    [string]$UpstreamRoot = '',
    [string]$OutputDirectory = '',
    [string]$Go = 'go'
)

$ErrorActionPreference = 'Stop'
$repo = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$buildRoot = Join-Path $repo '.tmp/release-build'
$output = if ($OutputDirectory) { $OutputDirectory } else { Join-Path $repo 'dist' }
New-Item -ItemType Directory -Path $buildRoot, $output -Force | Out-Null
$output = (Resolve-Path $output).Path
$packer = Join-Path $PSScriptRoot 'tarpack.go'
$packages = @()

function Copy-Overlay {
    param([string]$Source, [string]$Destination)
    Get-ChildItem -LiteralPath $Source -Recurse -File | ForEach-Object {
        $relative = [System.IO.Path]::GetRelativePath($Source, $_.FullName)
        $target = Join-Path $Destination $relative
        New-Item -ItemType Directory -Path (Split-Path $target -Parent) -Force | Out-Null
        Copy-Item -LiteralPath $_.FullName -Destination $target -Force
    }
}

function Find-UpstreamDirectory {
    param([string]$Architecture)
    $roots = @()
    if ($UpstreamRoot) {
        $roots += $UpstreamRoot
    }
    $roots += $repo
    foreach ($root in $roots) {
        foreach ($candidate in @(
            (Join-Path $root "prometheus-$Version.linux-$Architecture"),
            (Join-Path $root "extract-$Architecture/prometheus-$Version.linux-$Architecture")
        )) {
            if ((Test-Path -LiteralPath (Join-Path $candidate 'prometheus')) -and (Test-Path -LiteralPath (Join-Path $candidate 'promtool'))) {
                return (Resolve-Path $candidate).Path
            }
        }
    }
    throw "Prometheus $Version binaries for $Architecture were not found under '$UpstreamRoot'."
}

foreach ($architecture in $Architectures) {
    $manifestArch = if ($architecture -eq 'amd64') { 'x86_64' } else { 'arm' }
    $platform = if ($architecture -eq 'amd64') { 'x86' } else { 'arm' }
    $upstream = Find-UpstreamDirectory -Architecture $architecture
    $managerBinary = Join-Path $buildRoot "prometheus-manager-$architecture"

    $env:CGO_ENABLED = '0'
    $env:GOOS = 'linux'
    $env:GOARCH = $architecture
    Push-Location (Join-Path $repo 'manager')
    try {
        & $Go build -trimpath '-ldflags=-s -w' -o $managerBinary .
        if ($LASTEXITCODE -ne 0) { throw "Go build failed for $architecture" }
    } finally {
        Pop-Location
        Remove-Item Env:CGO_ENABLED, Env:GOOS, Env:GOARCH -ErrorAction SilentlyContinue
    }

    foreach ($variant in @('control', 'upgrade')) {
        $stage = Join-Path $buildRoot "$variant-$architecture"
        Remove-Item -LiteralPath $stage -Recurse -Force -ErrorAction SilentlyContinue
        New-Item -ItemType Directory -Path $stage -Force | Out-Null
        Copy-Item -LiteralPath (Join-Path $repo 'app') -Destination (Join-Path $stage 'app') -Recurse
        Copy-Item -LiteralPath (Join-Path $repo 'ui') -Destination (Join-Path $stage 'ui') -Recurse
        Copy-Item -LiteralPath (Join-Path $repo 'prometheus') -Destination (Join-Path $stage 'prometheus') -Recurse

        if ($variant -eq 'upgrade') {
            Copy-Overlay -Source (Join-Path $repo 'packaging/upgrade/app') -Destination (Join-Path $stage 'app')
            Copy-Overlay -Source (Join-Path $repo 'packaging/upgrade/prometheus') -Destination (Join-Path $stage 'prometheus')
            Remove-Item -LiteralPath (Join-Path $stage 'prometheus/cmd/migrate_from_original') -Force
        }

        Copy-Item -LiteralPath (Join-Path $upstream 'prometheus') -Destination (Join-Path $stage 'app/prometheus')
        Copy-Item -LiteralPath (Join-Path $upstream 'promtool') -Destination (Join-Path $stage 'app/promtool')
        Copy-Item -LiteralPath $managerBinary -Destination (Join-Path $stage 'app/prometheus-manager')

        $manifestPath = Join-Path $stage 'prometheus/manifest'
        $manifest = Get-Content -Raw -LiteralPath $manifestPath
        $manifest = $manifest.Replace('target_version', $Version).Replace('target_pack_arch', $architecture).Replace('this_pack_version', $PackVersion).Replace('this_pack_arch', $manifestArch).Replace('this_pack_platform', $platform)
        [System.IO.File]::WriteAllText($manifestPath, $manifest, [System.Text.UTF8Encoding]::new($false))

        $appArchive = Join-Path $stage 'prometheus/app.tgz'
        & $Go run $packer -root $stage -output $appArchive app ui
        if ($LASTEXITCODE -ne 0) { throw "app.tgz failed for $variant/$architecture" }

        $applicationID = if ($variant -eq 'control') { 'prometheus.control' } else { 'prometheus.prometheus' }
        $packagePath = Join-Path $output "appstore.$applicationID.$PackVersion.$architecture.fpk"
        & $Go run $packer -root $stage -output $packagePath prometheus
        if ($LASTEXITCODE -ne 0) { throw "FPK failed for $variant/$architecture" }
        $packages += $packagePath

        [pscustomobject]@{
            Variant = $variant
            Architecture = $architecture
            Package = $packagePath
            MiB = [math]::Round((Get-Item -LiteralPath $packagePath).Length / 1MB, 2)
        }
    }
}

$checksumLines = foreach ($package in $packages) {
    $hash = (Get-FileHash -Algorithm SHA256 -LiteralPath $package).Hash.ToLowerInvariant()
    "$hash  $([System.IO.Path]::GetFileName($package))"
}
[System.IO.File]::WriteAllLines((Join-Path $output 'SHA256SUMS.txt'), $checksumLines, [System.Text.UTF8Encoding]::new($false))
