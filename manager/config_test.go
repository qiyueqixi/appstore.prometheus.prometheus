package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPatchJobsPreservesAdvancedFields(t *testing.T) {
	document, err := parseYAMLDocument([]byte(`global:
  scrape_interval: 15s
scrape_configs:
  - job_name: node
    honor_labels: true
    static_configs:
      - targets: ["127.0.0.1:9100"]
        custom_field: keep-me
`))
	if err != nil {
		t.Fatal(err)
	}
	jobIndex := 0
	staticIndex := 0
	err = patchJobs(document, []jobView{{
		SourceIndex: &jobIndex,
		JobName:     "node-exporter",
		StaticConfigs: []staticConfigView{{
			SourceIndex: &staticIndex,
			Targets:     []string{"192.168.1.20:9100"},
		}},
	}})
	if err != nil {
		t.Fatal(err)
	}

	job := mappingValue(document.Content[0], "scrape_configs").Content[0]
	if value := mappingValue(job, "honor_labels"); value == nil || value.Value != "true" {
		t.Fatalf("advanced job field was not preserved: %#v", value)
	}
	staticConfig := mappingValue(job, "static_configs").Content[0]
	if value := mappingValue(staticConfig, "custom_field"); value == nil || value.Value != "keep-me" {
		t.Fatalf("advanced static field was not preserved: %#v", value)
	}
	view := readJobs(document)
	if view[0].JobName != "node-exporter" || view[0].StaticConfigs[0].Targets[0] != "192.168.1.20:9100" {
		t.Fatalf("job was not patched: %#v", view[0])
	}
}

func TestDeviceAliasesRoundTripAndPreserveOtherRelabelRules(t *testing.T) {
	document, err := parseYAMLDocument([]byte(`scrape_configs:
  - job_name: node
    static_configs:
      - targets: ["192.168.1.20:9100"]
    relabel_configs:
      - source_labels: [__address__]
        regex: "192.168.1.20:9100"
        target_label: alias
        replacement: old-name
      - source_labels: [__address__]
        regex: ".*:9200"
        action: drop
`))
	if err != nil {
		t.Fatal(err)
	}

	jobs := readJobs(document)
	if len(jobs) != 1 || len(jobs[0].StaticConfigs) != 1 || len(jobs[0].StaticConfigs[0].Devices) != 1 {
		t.Fatalf("devices were not read: %#v", jobs)
	}
	if alias := jobs[0].StaticConfigs[0].Devices[0].Alias; alias != "old-name" {
		t.Fatalf("unexpected device alias: %q", alias)
	}
	if !jobs[0].Advanced {
		t.Fatal("non-alias relabel rule should keep the advanced marker")
	}

	jobs[0].StaticConfigs[0].Devices = []deviceView{
		{Address: "192.168.1.20:9100", Alias: "new-name"},
		{Address: "192.168.1.21:9100", Alias: "second-node"},
	}
	if err := patchJobs(document, jobs); err != nil {
		t.Fatal(err)
	}

	job := mappingValue(document.Content[0], "scrape_configs").Content[0]
	rules := mappingValue(job, "relabel_configs")
	if rules == nil || len(rules.Content) != 3 {
		t.Fatalf("unexpected relabel rules: %#v", rules)
	}
	if action := scalarValue(mappingValue(rules.Content[0], "action")); action != "drop" {
		t.Fatalf("advanced relabel rule was not preserved: %q", action)
	}

	roundTrip := readJobs(document)
	devices := roundTrip[0].StaticConfigs[0].Devices
	if len(devices) != 2 || devices[0].Alias != "new-name" || devices[1].Alias != "second-node" {
		t.Fatalf("device aliases did not round trip: %#v", devices)
	}
	if targets := roundTrip[0].StaticConfigs[0].Targets; len(targets) != 2 || targets[1] != "192.168.1.21:9100" {
		t.Fatalf("device targets did not round trip: %#v", targets)
	}
}

func TestAliasOnlyRelabelRulesAreEditableInVisualMode(t *testing.T) {
	document, err := parseYAMLDocument([]byte(`scrape_configs:
  - job_name: node
    static_configs:
      - targets: ["192.168.1.20:9100"]
    relabel_configs:
      - source_labels: [__address__]
        regex: "192.168.1.20:9100"
        target_label: alias
        replacement: node-20
`))
	if err != nil {
		t.Fatal(err)
	}
	jobs := readJobs(document)
	if jobs[0].Advanced {
		t.Fatal("standard device alias rules should not be marked advanced")
	}
}

func TestDeviceOrderWritesTargetsAndAliasRulesInTheSameOrder(t *testing.T) {
	document, err := parseYAMLDocument([]byte(`scrape_configs:
  - job_name: node
    static_configs:
      - targets: ["192.168.1.10:9100"]
`))
	if err != nil {
		t.Fatal(err)
	}

	jobs := readJobs(document)
	jobs[0].StaticConfigs[0].Devices = []deviceView{
		{Address: "192.168.1.30:9100", Alias: "third"},
		{Address: "192.168.1.10:9100", Alias: "first"},
		{Address: "192.168.1.20:9100", Alias: "second"},
	}
	if err := patchJobs(document, jobs); err != nil {
		t.Fatal(err)
	}

	job := mappingValue(document.Content[0], "scrape_configs").Content[0]
	staticConfig := mappingValue(job, "static_configs").Content[0]
	targets := mappingValue(staticConfig, "targets")
	wantAddresses := []string{"192.168.1.30:9100", "192.168.1.10:9100", "192.168.1.20:9100"}
	if targets == nil || len(targets.Content) != len(wantAddresses) {
		t.Fatalf("unexpected targets: %#v", targets)
	}
	for index, want := range wantAddresses {
		if got := targets.Content[index].Value; got != want {
			t.Fatalf("target %d order mismatch: got %q want %q", index, got, want)
		}
	}

	rules := mappingValue(job, "relabel_configs")
	wantAliases := []string{"third", "first", "second"}
	if rules == nil || len(rules.Content) != len(wantAliases) {
		t.Fatalf("unexpected alias rules: %#v", rules)
	}
	for index, want := range wantAliases {
		if got := scalarValue(mappingValue(rules.Content[index], "replacement")); got != want {
			t.Fatalf("alias rule %d order mismatch: got %q want %q", index, got, want)
		}
	}
}

func TestSaveBasicCreatesBackupAndRejectsStaleChecksum(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(directory, "prometheus.yml")
	initial := []byte("global:\n  scrape_interval: 15s\nscrape_configs: []\n")
	if err := os.WriteFile(configPath, initial, 0600); err != nil {
		t.Fatal(err)
	}

	store := &configStore{
		path:        configPath,
		backupLimit: 20,
		validateFunc: func(content []byte) (string, error) {
			if !strings.Contains(string(content), "scrape_interval: 30s") {
				t.Fatal("validator did not receive updated config")
			}
			return "SUCCESS", nil
		},
	}
	result, view, err := store.saveBasic(basicConfigRequest{
		Checksum: checksum(initial),
		Global: globalView{
			ScrapeInterval:     "30s",
			EvaluationInterval: "30s",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.BackupPath == "" || len(view.Backups) != 1 {
		t.Fatalf("backup was not created: %#v %#v", result, view.Backups)
	}
	updated, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(updated), "scrape_interval: 30s") {
		t.Fatalf("config was not updated: %s", updated)
	}
	_, _, err = store.saveBasic(basicConfigRequest{Checksum: checksum(initial)})
	if !errors.Is(err, errConfigChanged) {
		t.Fatalf("expected stale checksum error, got %v", err)
	}
}

func TestBackupSettingsAndDeleteBackup(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(directory, "prometheus.yml")
	if err := os.WriteFile(configPath, []byte("global:\n"), 0600); err != nil {
		t.Fatal(err)
	}
	store := &configStore{path: configPath, backupLimit: 20}

	view, err := store.saveBackupSettings(backupSettingsRequest{Enabled: false, Retention: 2})
	if err != nil {
		t.Fatal(err)
	}
	if view.BackupSettings.Enabled || view.BackupSettings.Retention != 2 {
		t.Fatalf("unexpected backup settings: %#v", view.BackupSettings)
	}
	if view.BackupDirectory != filepath.Join(directory, ".prometheus-backups") {
		t.Fatalf("unexpected backup directory: %q", view.BackupDirectory)
	}

	if err := os.MkdirAll(store.backupDirectory(), 0700); err != nil {
		t.Fatal(err)
	}
	backupName := "prometheus.yml.20260827-120000.000000000"
	backupPath := filepath.Join(store.backupDirectory(), backupName)
	if err := os.WriteFile(backupPath, []byte("global:\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.deleteBackup(backupName); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(backupPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("backup was not deleted: %v", err)
	}
	if _, err := store.deleteBackup("../prometheus.yml"); err == nil {
		t.Fatal("path traversal backup name was accepted")
	}
}

func TestInvalidConfigIsNotWritten(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(directory, "prometheus.yml")
	initial := []byte("global:\n  scrape_interval: 15s\n")
	if err := os.WriteFile(configPath, initial, 0600); err != nil {
		t.Fatal(err)
	}
	store := &configStore{
		path:        configPath,
		backupLimit: 20,
		validateFunc: func(_ []byte) (string, error) {
			return "bad duration", errors.New("validation failed")
		},
	}
	result, _, err := store.saveRaw(rawConfigRequest{Checksum: checksum(initial), Raw: "global:\n  scrape_interval: nope\n"})
	if err == nil || result.Output != "bad duration" {
		t.Fatalf("expected validation error, got result=%#v err=%v", result, err)
	}
	actual, readErr := os.ReadFile(configPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(actual) != string(initial) {
		t.Fatalf("invalid config replaced the original: %s", actual)
	}
}

func TestProfilesInitializeFromExistingConfiguration(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(directory, "prometheus.yml")
	initial := []byte(`global:
  scrape_interval: 5s
scrape_configs:
  - job_name: prometheus
    static_configs:
      - targets: ["127.0.0.1:9090"]
  - job_name: node
    static_configs:
      - targets: ["192.168.1.20:9100"]
`)
	if err := os.WriteFile(configPath, initial, 0600); err != nil {
		t.Fatal(err)
	}
	store := &configStore{path: configPath, backupLimit: 20}

	view, err := store.load()
	if err != nil {
		t.Fatal(err)
	}
	if view.Profile != "prometheus" || view.ActiveProfile != "prometheus" {
		t.Fatalf("unexpected active profile: %#v", view)
	}
	if len(view.Profiles) != 1 || !view.Profiles[0].Active || view.Profiles[0].FileName != "prometheus.yml" {
		t.Fatalf("unexpected profile list: %#v", view.Profiles)
	}
	if len(view.Jobs) != 2 || view.Jobs[0].JobName != "prometheus" || view.Jobs[1].JobName != "node" {
		t.Fatalf("existing jobs were not preserved: %#v", view.Jobs)
	}
	profileContent, err := os.ReadFile(filepath.Join(directory, "configs", "prometheus.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(profileContent) != string(initial) {
		t.Fatalf("initial profile does not match existing config:\n%s", profileContent)
	}
}

func TestCreateProfileUsesTwoJobTemplateAndPreservesDisplayName(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(directory, "prometheus.yml")
	if err := os.WriteFile(configPath, []byte("scrape_configs: []\n"), 0600); err != nil {
		t.Fatal(err)
	}
	store := &configStore{
		path: configPath,
		validateFunc: func(content []byte) (string, error) {
			if _, err := parseYAMLDocument(content); err != nil {
				return "", err
			}
			return "SUCCESS", nil
		},
	}

	view, err := store.createProfile(createProfileRequest{Name: "机房设备"})
	if err != nil {
		t.Fatal(err)
	}
	if view.Profile != "config" || view.ActiveProfile != "prometheus" {
		t.Fatalf("new profile selection is incorrect: %#v", view)
	}
	if len(view.Jobs) != 2 || view.Jobs[0].JobName != "prometheus" || view.Jobs[1].JobName != "node" {
		t.Fatalf("new profile template should contain local and device jobs: %#v", view.Jobs)
	}
	if targets := view.Jobs[0].StaticConfigs[0].Targets; len(targets) != 1 || targets[0] != "127.0.0.1:19091" {
		t.Fatalf("unexpected local target: %#v", targets)
	}
	if targets := view.Jobs[1].StaticConfigs[0].Targets; len(targets) != 0 {
		t.Fatalf("device job should start empty: %#v", targets)
	}
	found := false
	for _, profile := range view.Profiles {
		if profile.ID == "config" {
			found = profile.Name == "机房设备" && !profile.Active
		}
	}
	if !found {
		t.Fatalf("new profile name was not preserved: %#v", view.Profiles)
	}
}

func TestCreateProfileUsesConfiguredLocalPrometheusTarget(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(directory, "prometheus.yml")
	if err := os.WriteFile(configPath, []byte("scrape_configs: []\n"), 0600); err != nil {
		t.Fatal(err)
	}
	store := &configStore{
		path:                    configPath,
		defaultPrometheusTarget: "127.0.0.1:9091",
		validateFunc: func(content []byte) (string, error) {
			if _, err := parseYAMLDocument(content); err != nil {
				return "", err
			}
			return "SUCCESS", nil
		},
	}

	view, err := store.createProfile(createProfileRequest{Name: "upgrade"})
	if err != nil {
		t.Fatal(err)
	}
	if targets := view.Jobs[0].StaticConfigs[0].Targets; len(targets) != 1 || targets[0] != "127.0.0.1:9091" {
		t.Fatalf("unexpected local target: %#v", targets)
	}
}

func TestRenameProfileChangesDisplayNameOnly(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(directory, "prometheus.yml")
	if err := os.WriteFile(configPath, []byte("scrape_configs: []\n"), 0600); err != nil {
		t.Fatal(err)
	}
	store := &configStore{path: configPath, validateFunc: func(content []byte) (string, error) { return "SUCCESS", nil }}
	created, err := store.createProfile(createProfileRequest{Name: "secondary"})
	if err != nil {
		t.Fatal(err)
	}
	renamed, err := store.renameProfile(created.Profile, renameProfileRequest{Name: "机房采集"})
	if err != nil {
		t.Fatal(err)
	}
	if renamed.Profile != created.Profile {
		t.Fatalf("rename changed profile id: %q", renamed.Profile)
	}
	if _, err := os.Stat(filepath.Join(directory, "configs", created.Profile+".yml")); err != nil {
		t.Fatalf("rename changed physical configuration file: %v", err)
	}
	found := false
	for _, profile := range renamed.Profiles {
		if profile.ID == created.Profile {
			found = profile.Name == "机房采集" && profile.FileName == created.Profile+".yml"
		}
	}
	if !found {
		t.Fatalf("renamed profile metadata not found: %#v", renamed.Profiles)
	}
}

func TestDeleteProfileRejectsActiveAndDeletesInactive(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(directory, "prometheus.yml")
	if err := os.WriteFile(configPath, []byte("scrape_configs: []\n"), 0600); err != nil {
		t.Fatal(err)
	}
	store := &configStore{path: configPath, validateFunc: func(content []byte) (string, error) { return "SUCCESS", nil }}
	created, err := store.createProfile(createProfileRequest{Name: "secondary"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.deleteProfile("prometheus"); err == nil || !strings.Contains(err.Error(), "running configuration") {
		t.Fatalf("active profile deletion should be rejected, got %v", err)
	}
	view, err := store.deleteProfile(created.Profile)
	if err != nil {
		t.Fatal(err)
	}
	if view.Profile != "prometheus" || len(view.Profiles) != 1 {
		t.Fatalf("unexpected view after deletion: %#v", view)
	}
	if _, err := os.Stat(filepath.Join(directory, "configs", created.Profile+".yml")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("inactive profile still exists: %v", err)
	}
}

func TestDeleteProfileRejectsLastConfiguration(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(directory, "prometheus.yml")
	if err := os.WriteFile(configPath, []byte("scrape_configs: []\n"), 0600); err != nil {
		t.Fatal(err)
	}
	store := &configStore{path: configPath, validateFunc: func(content []byte) (string, error) { return "SUCCESS", nil }}
	if _, err := store.deleteProfile("prometheus"); err == nil || !strings.Contains(err.Error(), "last configuration") {
		t.Fatalf("last configuration deletion should be rejected, got %v", err)
	}
}

func TestApplyingInactiveProfileUpdatesCanonicalConfiguration(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(directory, "prometheus.yml")
	initial := []byte("global:\n  scrape_interval: 15s\nscrape_configs: []\n")
	if err := os.WriteFile(configPath, initial, 0600); err != nil {
		t.Fatal(err)
	}
	store := &configStore{
		path: configPath,
		validateFunc: func(content []byte) (string, error) {
			if _, err := parseYAMLDocument(content); err != nil {
				return "", err
			}
			return "SUCCESS", nil
		},
	}
	created, err := store.createProfile(createProfileRequest{Name: "secondary"})
	if err != nil {
		t.Fatal(err)
	}
	next := strings.Replace(created.Raw, "scrape_interval: 15s", "scrape_interval: 30s", 1)
	_, applied, err := store.saveRaw(rawConfigRequest{Profile: created.Profile, Checksum: created.Checksum, Raw: next})
	if err != nil {
		t.Fatal(err)
	}
	if applied.ActiveProfile != "secondary" || applied.Profile != "secondary" {
		t.Fatalf("profile was not activated: %#v", applied)
	}
	canonical, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(canonical), "scrape_interval: 30s") {
		t.Fatalf("canonical configuration was not replaced: %s", canonical)
	}
	originalProfile, err := os.ReadFile(filepath.Join(directory, "configs", "prometheus.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(originalProfile) != string(initial) {
		t.Fatalf("original profile changed while applying another profile: %s", originalProfile)
	}
	originalView, err := store.loadProfile("prometheus")
	if err != nil {
		t.Fatal(err)
	}
	if originalView.ActiveProfile != "secondary" || originalView.Profile != "prometheus" || originalView.Raw != string(initial) {
		t.Fatalf("inactive profile could not be selected: %#v", originalView)
	}
}
