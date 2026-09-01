# Prometheus Control for fnOS

Prometheus Control 是面向飞牛 fnOS 的 Prometheus 图形化管理版本。项目基于
[`fnnas/appstore.prometheus.prometheus`](https://github.com/fnnas/appstore.prometheus.prometheus)
改造，继续使用 Prometheus 官方二进制，同时增加一个完全由 Go 编写的图形化管理门户。每个架构提供“原版升级”和“独立安装”两个发行变体。

它不是 Prometheus 官方发行包。Prometheus 本体及 PromQL、TSDB、服务发现等核心能力仍来自
[`prometheus/prometheus`](https://github.com/prometheus/prometheus)。

## 选择安装包

| 项目 | 原版 fnOS 包 | 原版升级包 | 独立安装包 |
| --- | --- | --- | --- |
| 文件名 | 原版 FPK | `appstore.prometheus.prometheus.*.fpk` | `appstore.prometheus.control.*.fpk` |
| 应用 ID | `prometheus.prometheus` | `prometheus.prometheus` | `prometheus.control` |
| 用途 | 原版功能 | 直接替换原版并复用数据 | 作为第二个独立应用安装 |
| 对外端口 | `9090` | `9090` | `19090` |
| 内部 Prometheus | 直接监听 `9090` | `127.0.0.1:9091` | `127.0.0.1:19091` |
| 运行用户 | `prometheus` | `prometheus` | `prometheus-control` |
| 数据目录 | 原版目录 | 原版目录 | 独立目录 |
| 历史数据处理 | 已在原目录 | 无需迁移 | 使用随包脚本复制 |

只想在现有原版上增加图形化管理时，选择原版升级包。它使用相同应用 ID，不能与原版同时安装，因为安装行为就是升级原版。

希望保留原版应用、使用另一套端口和目录时，选择独立安装包。两个应用可以同时安装，但不得同时读写同一个 TSDB 目录。独立安装包不会自动修改、移动或删除原版数据。

## 卸载与数据保留

卸载时会显示中文的数据处理选项，默认选择“保留数据”。只有明确选择“删除全部数据（不可恢复）”后，卸载脚本才会删除对应版本的数据目录。

| 变体 | 保留或删除的数据目录 |
| --- | --- |
| 原版升级包 | `/var/apps/prometheus.prometheus/shares/prometheus/prometheus/` |
| 独立安装包 | `/var/apps/prometheus.control/shares/prometheus-control/` |

目录中包含 `prometheus.yml`、其他配置文件、TSDB `data/`、配置备份和运行日志。原版升级包与独立安装包分别处理自己的目录，不会跨应用删除数据。选择删除后无法恢复，重要数据应先另行备份。

## 主要功能

- 图形化编辑全局采集间隔、采集任务、静态目标、设备别名和公共标签
- 在同一应用中维护多个 YAML 配置文件，并明确标记当前运行的配置
- 支持拖动或方向键调整设备顺序，并按界面顺序写回 `targets` 与设备别名规则
- 保留原始 YAML 编辑器，兼容原版 Prometheus 配置及高级字段
- 保存前运行 `promtool check config`，校验失败时不覆盖现有配置
- 自动备份旧配置，可设置保留数量、查看备份位置并删除备份记录
- 校验通过后热重载 Prometheus，无需重启整个应用
- 在同一页面内嵌 Prometheus 原版 Web，也可在新窗口打开
- 反向代理 `/api/v1/*`、`/metrics`、`/federate` 和 `/-/*` 等原版接口
- 支持 Basic Auth、amd64 和 arm64
- 管理门户编译为单个 Go 二进制，运行和构建不依赖 Node.js

## 安装与访问

共同要求：

- fnOS `1.2.0000` 或更高版本
- 安装与 NAS CPU 架构匹配的 `amd64` 或 `arm64` 包

### 原版升级包

1. 先备份原版的 `prometheus.yml`、`prometheus-web.yml` 和整个 `data/` 目录。
2. 使用 `appstore.prometheus.prometheus.<版本>.<架构>.fpk` 更新已安装的原版应用。
3. 不要运行迁移脚本，也不要复制或合并 WAL；升级包会直接复用原目录。
4. 升级后继续访问 `http://NAS_IP:9090/`。

原版配置中的本机采集目标 `127.0.0.1:9090` 可以保留。升级后该地址会先到 Go 门户，再由兼容入口 `/metrics` 转发到内部 Prometheus。新建配置文件时则会直接使用 `127.0.0.1:9091`。

### 独立安装包

独立安装需要 `19090` 端口可用。如需迁移历史数据，目标存储空间应至少大于原版目录已用空间。

安装后访问：

```text
http://NAS_IP:19090/
```

常用路径：

- `/`：Prometheus Control 图形化配置门户
- `/prometheus/`：内嵌的 Prometheus 原版 Web
- `/api/v1/*`：Prometheus HTTP API 兼容入口
- `/metrics`：Prometheus 自身指标兼容入口

如果检测到原版 Prometheus 的配置和 TSDB 目录仍存在，门户顶部会显示迁移提示。提示只提供路径和安全步骤，不会在应用运行时自动复制 WAL。

外部请求由 Go 门户监听 `19090`；Prometheus 本体仅监听 `127.0.0.1:19091`，不会直接暴露到局域网。

## 数据和配置目录

| 变体 | fnOS 逻辑路径 | 数据卷实际路径 |
| --- | --- | --- |
| 原版升级包 | `/var/apps/prometheus.prometheus/shares/prometheus/prometheus/` | `/volX/@appshare/prometheus/prometheus/` |
| 独立安装包 | `/var/apps/prometheus.control/shares/prometheus-control/` | `/volX/@appshare/prometheus-control/` |

以下为独立安装包的逻辑路径：

```text
/var/apps/prometheus.control/shares/prometheus-control/
```

对应数据卷中的实际路径通常为：

```text
/volX/@appshare/prometheus-control/
```

主要文件：

```text
prometheus-control/
├── prometheus.yml
├── configs/
│   ├── prometheus.yml
│   └── <其他配置>.yml
├── .prometheus-config-profiles.json
├── data/
├── .prometheus-backups/
├── .prometheus-backup-settings.json
├── prometheus.log
└── prometheus-manager.log
```

独立安装包的 Web 认证配置位于：

```text
/var/apps/prometheus.control/etc/prometheus-web.yml
```

## 独立版迁移原版数据

本节只适用于 `appstore.prometheus.control.*.fpk` 独立安装包。`appstore.prometheus.prometheus.*.fpk` 原版升级包直接复用原目录，不需要也不包含迁移脚本。

配置文件和历史数据是两类内容：

- `prometheus.yml` 决定抓取任务、目标、标签和规则。
- `data/` 是 Prometheus TSDB，包含历史块、WAL 和当前 head 数据。

不能在原版 Prometheus 运行时复制 WAL，也不能让新旧两个 Prometheus 同时使用同一个 `data/`。迁移采用完整复制，因此原版数据仍会保留，但目标卷会额外占用一份空间。

> **不要手动合并目录。** 禁止使用 `cp`、`rsync` 或文件管理器将原版文件直接覆盖到 Control 已初始化的 `data/`。低编号的新 WAL 与原版高编号 WAL 混合后会触发 `segments are not sequential`，导致 Prometheus 无法启动。只使用下面的随包迁移脚本。

### 迁移步骤

1. 安装 Prometheus Control。
2. 在应用中心停止原版 `Prometheus`。
3. 在应用中心停止 `Prometheus Control`。
4. 通过 SSH 执行迁移脚本：

```bash
sudo /var/apps/prometheus.control/target/cmd/migrate_from_original
```

5. 脚本完成后，在应用中心启动 `Prometheus Control`，原版暂时保持停止。
6. 打开 `http://NAS_IP:19090/`，确认配置和历史查询正常。
7. 如果导入配置中存在 Prometheus 自身目标 `127.0.0.1:9090`，将它改为 `127.0.0.1:19091` 后校验并应用。
8. 确认历史数据、Grafana 查询和采集状态都正常后，再决定是否保留或卸载原版应用。

迁移脚本会执行以下操作：

- 检查执行用户为 `root`
- 检查新旧 Prometheus 是否都已停止
- 检查原版配置、TSDB、新应用和剩余磁盘空间
- 将原版应用目录复制到暂存目录
- 使用新包中的 `promtool` 校验复制后的配置
- 校验复制后的 WAL 和 `chunks_head` 编号是否连续
- 切换数据前再次确认两个 Prometheus 都没有在复制期间被启动
- 将新应用已有的初始文件移动到回滚目录
- 将配置、规则文件、配置备份、日志及完整 TSDB 写入新应用目录
- 如原版存在 `prometheus-web.yml`，同时复制原版认证配置
- 将新目录所有权调整为 `prometheus-control:prometheus-control`

迁移不会删除或修改：

```text
/var/apps/prometheus.prometheus/shares/prometheus/prometheus/
```

新应用原有文件的回滚目录会显示在脚本输出中，默认位于：

```text
/var/apps/prometheus.control/shares/prometheus-control/migration-rollback-YYYYMMDD-HHMMSS/
```

如果复制或配置校验失败，不要启动新应用覆盖现场。脚本会输出保留的暂存目录和具体错误，可先修复原版配置或检查磁盘空间后再处理。

## 默认配置

| 配置项 | 原版升级包 | 独立安装包 |
| --- | --- | --- |
| 外部管理端口 | `9090` | `19090` |
| 内部 Prometheus 端口 | `127.0.0.1:9091` | `127.0.0.1:19091` |
| 抓取间隔 | `15s` | `15s` |
| 规则评估间隔 | `15s` | `15s` |
| 数据保留时间 | `180d` | `180d` |

全新安装默认提供一个 `prometheus.yml` 配置文件，其中包含两个独立采集任务：

- `prometheus`：升级包默认抓取 `127.0.0.1:9091/metrics`，独立包默认抓取 `127.0.0.1:19091/metrics`
- `node`：使用标准 `/metrics` 路径，初始设备列表为空，由用户逐台添加 Node Exporter 地址

从原版迁移时会保留原来的 `scrape_configs` 结构，不会自动合并或重命名任务，避免改变 `job` 标签并影响现有 Grafana 查询。

## 配置保存流程

1. 门户把现有 `prometheus.yml` 登记为初始配置文件，并在 `configs/` 中保留对应副本。
2. 顶部可以新建和选择配置文件；一个配置文件内可以同时包含多个 `scrape_configs` 采集任务。
3. 保存时检查所选文件是否被其他程序修改，避免覆盖并发变更。
4. 使用随包提供的 `promtool` 校验所选配置。
5. 校验通过后备份当前运行配置，将所选文件复制为活动 `prometheus.yml`，并记录运行中的配置。
6. 调用 Prometheus `/-/reload` 热重载配置。

Prometheus 进程始终只读取活动的 `prometheus.yml`，不会同时加载 `configs/` 下的多个文件。切换顶部配置文件只会查看内容，点击“校验并应用”后才会将它设为运行配置。

图形化编辑只修改界面支持的常用字段。服务发现、复杂重标签和其他高级字段会继续保留，需要精确修改时可切换到“原始 YAML”。

## 本地开发

管理门户位于 `manager/`，只需要 Go：

```bash
cd manager
go test ./...
go build .
```

前端文件位于 `manager/web/`，通过 `go:embed` 编译进管理二进制，不需要 Node.js 或前端打包器。

## 构建 FPK

GitHub Actions 会：

1. 下载指定版本的 Prometheus 和 `promtool`。
2. 将 Go 管理门户静态交叉编译为 linux/amd64 或 linux/arm64。
3. 为每个架构生成原版升级和独立安装两种 FPK，并生成 SHA256 校验文件。

构建产物：

```text
appstore.prometheus.control.<包版本>.amd64.fpk
appstore.prometheus.control.<包版本>.arm64.fpk
appstore.prometheus.prometheus.<包版本>.amd64.fpk
appstore.prometheus.prometheus.<包版本>.arm64.fpk
SHA256SUMS.txt
```

本地已有 Prometheus 官方解压目录时，可运行：

```powershell
./scripts/package-release.ps1 -Version 3.13.0 -PackVersion 3.13.0-16 -UpstreamRoot <官方解压目录的父目录>
```

FPK 内部的 `app.tgz` 是 fnOS 应用格式的一部分，并非需要单独安装的发布文件。

可在仓库 Actions 页面手动运行 `Test Release` 工作流。

## 上游项目

- [Prometheus 官方文档](https://prometheus.io/docs/)
- [Prometheus 源码](https://github.com/prometheus/prometheus)
- [原 fnOS 打包项目](https://github.com/fnnas/appstore.prometheus.prometheus)
- [飞牛 fnOS](https://www.fnnas.com/)
