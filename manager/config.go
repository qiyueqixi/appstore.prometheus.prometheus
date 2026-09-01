package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

var errConfigChanged = errors.New("configuration changed since it was loaded")

const defaultPrometheusTarget = "127.0.0.1:19091"

const defaultProfileYAMLTemplate = `# my global config
global:
  scrape_interval: 15s
  evaluation_interval: 15s

# Alertmanager configuration
alerting:
  alertmanagers:
    - static_configs:
        - targets:
          # - alertmanager:9093

# Load rules once and periodically evaluate them according to the global 'evaluation_interval'.
rule_files:
  # - "first_rules.yml"
  # - "second_rules.yml"

# Scrape configuration
scrape_configs:
  # Prometheus local service
  - job_name: "prometheus"
    metrics_path: "/metrics"
    static_configs:
      - targets: ["{{PROMETHEUS_TARGET}}"]
        labels:
          app: "prometheus"

  # User devices running node_exporter
  - job_name: "node"
    metrics_path: "/metrics"
    static_configs:
      - targets: []
`

type configStore struct {
	path                    string
	promtoolPath            string
	backupLimit             int
	defaultPrometheusTarget string
	validateFunc            func([]byte) (string, error)
	mu                      sync.Mutex
}

func (store *configStore) defaultProfileContent() []byte {
	target := strings.TrimSpace(store.defaultPrometheusTarget)
	if target == "" {
		target = defaultPrometheusTarget
	}
	return []byte(strings.ReplaceAll(defaultProfileYAMLTemplate, "{{PROMETHEUS_TARGET}}", target))
}

type configView struct {
	Checksum        string             `json:"checksum"`
	Profile         string             `json:"profile"`
	ActiveProfile   string             `json:"activeProfile"`
	Profiles        []profileView      `json:"profiles"`
	Raw             string             `json:"raw"`
	Global          globalView         `json:"global"`
	Jobs            []jobView          `json:"jobs"`
	Modified        string             `json:"modified"`
	Backups         []backupView       `json:"backups"`
	BackupSettings  backupSettingsView `json:"backupSettings"`
	BackupDirectory string             `json:"backupDirectory"`
}

type profileView struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	FileName string `json:"fileName"`
	Active   bool   `json:"active"`
	Modified string `json:"modified"`
}

type profileState struct {
	Active string            `json:"active"`
	Names  map[string]string `json:"names,omitempty"`
}

type createProfileRequest struct {
	Name string `json:"name"`
}

type renameProfileRequest struct {
	Name string `json:"name"`
}

type globalView struct {
	ScrapeInterval     string `json:"scrapeInterval"`
	EvaluationInterval string `json:"evaluationInterval"`
	ScrapeTimeout      string `json:"scrapeTimeout"`
}

type jobView struct {
	SourceIndex    *int               `json:"sourceIndex,omitempty"`
	JobName        string             `json:"jobName"`
	MetricsPath    string             `json:"metricsPath"`
	Scheme         string             `json:"scheme"`
	ScrapeInterval string             `json:"scrapeInterval"`
	ScrapeTimeout  string             `json:"scrapeTimeout"`
	StaticConfigs  []staticConfigView `json:"staticConfigs"`
	Advanced       bool               `json:"advanced"`
}

type staticConfigView struct {
	SourceIndex *int              `json:"sourceIndex,omitempty"`
	Targets     []string          `json:"targets"`
	Devices     []deviceView      `json:"devices"`
	Labels      map[string]string `json:"labels"`
	Advanced    bool              `json:"advanced"`
}

type deviceView struct {
	Address string `json:"address"`
	Alias   string `json:"alias"`
}

type deviceAliasRule struct {
	Pattern     string
	Replacement string
}

type basicConfigRequest struct {
	Checksum string     `json:"checksum"`
	Profile  string     `json:"profile"`
	Global   globalView `json:"global"`
	Jobs     []jobView  `json:"jobs"`
}

type rawConfigRequest struct {
	Checksum string `json:"checksum"`
	Profile  string `json:"profile"`
	Raw      string `json:"raw"`
}

type backupSettingsRequest struct {
	Enabled   bool `json:"enabled"`
	Retention int  `json:"retention"`
}

type backupSettingsView struct {
	Enabled   bool `json:"enabled"`
	Retention int  `json:"retention"`
}

type backupSettings struct {
	Enabled   bool `json:"enabled"`
	Retention int  `json:"retention"`
}

type backupView struct {
	Name     string `json:"name"`
	Modified string `json:"modified"`
	Size     int64  `json:"size"`
}

type saveResult struct {
	BackupPath string
	Output     string
}

func (store *configStore) load() (configView, error) {
	return store.loadProfile("")
}

func (store *configStore) loadProfile(profileID string) (configView, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	state, err := store.ensureProfilesUnlocked()
	if err != nil {
		return configView{}, err
	}
	return store.loadProfileUnlocked(profileID, state)
}

func (store *configStore) loadUnlocked() (configView, error) {
	state, err := store.ensureProfilesUnlocked()
	if err != nil {
		return configView{}, err
	}
	return store.loadProfileUnlocked(state.Active, state)
}

func (store *configStore) loadProfileUnlocked(profileID string, state profileState) (configView, error) {
	if profileID == "" {
		profileID = state.Active
	}
	path, err := store.profileReadPathUnlocked(profileID, state)
	if err != nil {
		return configView{}, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return configView{}, fmt.Errorf("read config: %w", err)
	}
	document, err := parseYAMLDocument(raw)
	if err != nil {
		return configView{}, fmt.Errorf("parse config: %w", err)
	}

	view := configView{
		Checksum:        checksum(raw),
		Profile:         profileID,
		ActiveProfile:   state.Active,
		Raw:             string(raw),
		Global:          readGlobal(document),
		Jobs:            readJobs(document),
		BackupDirectory: store.backupDirectory(),
	}
	if info, statErr := os.Stat(path); statErr == nil {
		view.Modified = info.ModTime().Format(time.RFC3339)
	}
	view.Profiles, err = store.listProfilesUnlocked(state)
	if err != nil {
		return configView{}, err
	}
	settings, err := store.loadBackupSettingsUnlocked()
	if err != nil {
		return configView{}, err
	}
	view.BackupSettings = backupSettingsView{Enabled: settings.Enabled, Retention: settings.Retention}
	view.Backups, _ = store.listBackupsUnlocked()
	return view, nil
}

func (store *configStore) saveBasic(request basicConfigRequest) (saveResult, configView, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	state, err := store.ensureProfilesUnlocked()
	if err != nil {
		return saveResult{}, configView{}, err
	}
	profileID := request.Profile
	if profileID == "" {
		profileID = state.Active
	}
	path, err := store.profileReadPathUnlocked(profileID, state)
	if err != nil {
		return saveResult{}, configView{}, err
	}
	current, err := os.ReadFile(path)
	if err != nil {
		return saveResult{}, configView{}, fmt.Errorf("read config: %w", err)
	}
	if request.Checksum != checksum(current) {
		return saveResult{}, configView{}, errConfigChanged
	}
	document, err := parseYAMLDocument(current)
	if err != nil {
		return saveResult{}, configView{}, fmt.Errorf("parse config: %w", err)
	}
	patchGlobal(document, request.Global)
	if err := patchJobs(document, request.Jobs); err != nil {
		return saveResult{}, configView{}, err
	}

	var output bytes.Buffer
	encoder := yaml.NewEncoder(&output)
	encoder.SetIndent(2)
	if err := encoder.Encode(document); err != nil {
		return saveResult{}, configView{}, fmt.Errorf("encode config: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return saveResult{}, configView{}, fmt.Errorf("close encoder: %w", err)
	}

	result, state, err := store.validateAndApplyProfileUnlocked(profileID, output.Bytes(), state)
	if err != nil {
		return result, configView{}, err
	}
	view, err := store.loadProfileUnlocked(profileID, state)
	return result, view, err
}

func (store *configStore) saveRaw(request rawConfigRequest) (saveResult, configView, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	state, err := store.ensureProfilesUnlocked()
	if err != nil {
		return saveResult{}, configView{}, err
	}
	profileID := request.Profile
	if profileID == "" {
		profileID = state.Active
	}
	path, err := store.profileReadPathUnlocked(profileID, state)
	if err != nil {
		return saveResult{}, configView{}, err
	}
	current, err := os.ReadFile(path)
	if err != nil {
		return saveResult{}, configView{}, fmt.Errorf("read config: %w", err)
	}
	if request.Checksum != checksum(current) {
		return saveResult{}, configView{}, errConfigChanged
	}
	if _, err := parseYAMLDocument([]byte(request.Raw)); err != nil {
		return saveResult{}, configView{}, fmt.Errorf("parse config: %w", err)
	}
	result, state, err := store.validateAndApplyProfileUnlocked(profileID, []byte(request.Raw), state)
	if err != nil {
		return result, configView{}, err
	}
	view, err := store.loadProfileUnlocked(profileID, state)
	return result, view, err
}

func (store *configStore) createProfile(request createProfileRequest) (configView, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	state, err := store.ensureProfilesUnlocked()
	if err != nil {
		return configView{}, err
	}
	profileID, err := store.availableProfileIDUnlocked(request.Name)
	if err != nil {
		return configView{}, err
	}
	path := store.profilePath(profileID)
	content := store.defaultProfileContent()
	if _, err := store.validate(content); err != nil {
		return configView{}, fmt.Errorf("validate default profile: %w", err)
	}
	if err := os.WriteFile(path, content, 0600); err != nil {
		return configView{}, fmt.Errorf("write profile: %w", err)
	}
	if state.Names == nil {
		state.Names = make(map[string]string)
	}
	state.Names[profileID] = strings.TrimSpace(request.Name)
	if err := store.saveProfileStateUnlocked(state); err != nil {
		_ = os.Remove(path)
		return configView{}, err
	}
	return store.loadProfileUnlocked(profileID, state)
}

func (store *configStore) renameProfile(profileID string, request renameProfileRequest) (configView, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	state, err := store.ensureProfilesUnlocked()
	if err != nil {
		return configView{}, err
	}
	if _, err := store.profileReadPathUnlocked(profileID, state); err != nil {
		return configView{}, err
	}
	name, err := normalizeProfileName(request.Name)
	if err != nil {
		return configView{}, err
	}
	state.Names[profileID] = name
	if err := store.saveProfileStateUnlocked(state); err != nil {
		return configView{}, err
	}
	return store.loadProfileUnlocked(profileID, state)
}

func (store *configStore) deleteProfile(profileID string) (configView, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	state, err := store.ensureProfilesUnlocked()
	if err != nil {
		return configView{}, err
	}
	if !validProfileID(profileID) {
		return configView{}, errors.New("invalid configuration file")
	}
	profiles, err := store.listProfilesUnlocked(state)
	if err != nil {
		return configView{}, err
	}
	if len(profiles) <= 1 {
		return configView{}, errors.New("the last configuration file cannot be deleted")
	}
	if profileID == state.Active {
		return configView{}, errors.New("running configuration cannot be deleted; apply another configuration first")
	}
	path := store.profilePath(profileID)
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return configView{}, fmt.Errorf("configuration file %q: %w", profileID, os.ErrNotExist)
	}
	if err != nil {
		return configView{}, fmt.Errorf("inspect configuration file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return configView{}, errors.New("configuration file is not a regular file")
	}

	previousName, hadName := state.Names[profileID]
	delete(state.Names, profileID)
	if err := store.saveProfileStateUnlocked(state); err != nil {
		return configView{}, err
	}
	if err := os.Remove(path); err != nil {
		if hadName {
			state.Names[profileID] = previousName
		}
		_ = store.saveProfileStateUnlocked(state)
		return configView{}, fmt.Errorf("delete configuration file: %w", err)
	}
	return store.loadProfileUnlocked(state.Active, state)
}

func (store *configStore) profilesDirectory() string {
	return filepath.Join(filepath.Dir(store.path), "configs")
}

func (store *configStore) profileStatePath() string {
	return filepath.Join(filepath.Dir(store.path), ".prometheus-config-profiles.json")
}

func (store *configStore) profilePath(profileID string) string {
	return filepath.Join(store.profilesDirectory(), profileID+".yml")
}

func (store *configStore) ensureProfilesUnlocked() (profileState, error) {
	directory := store.profilesDirectory()
	if err := os.MkdirAll(directory, 0700); err != nil {
		return profileState{}, fmt.Errorf("create profiles directory: %w", err)
	}

	state := profileState{Active: "prometheus", Names: map[string]string{"prometheus": "prometheus"}}
	content, err := os.ReadFile(store.profileStatePath())
	if err == nil {
		if unmarshalErr := json.Unmarshal(content, &state); unmarshalErr != nil {
			return profileState{}, fmt.Errorf("parse profile state: %w", unmarshalErr)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return profileState{}, fmt.Errorf("read profile state: %w", err)
	}
	if state.Active == "" || !validProfileID(state.Active) {
		state.Active = "prometheus"
	}
	if state.Names == nil {
		state.Names = make(map[string]string)
	}
	if state.Names["prometheus"] == "" {
		state.Names["prometheus"] = "prometheus"
	}

	activeContent, readErr := os.ReadFile(store.path)
	if errors.Is(readErr, os.ErrNotExist) {
		activeContent = store.defaultProfileContent()
		if writeErr := atomicWriteFile(store.path, activeContent); writeErr != nil {
			return profileState{}, fmt.Errorf("create active config: %w", writeErr)
		}
	} else if readErr != nil {
		return profileState{}, fmt.Errorf("read active config: %w", readErr)
	}

	activeProfilePath := store.profilePath(state.Active)
	profileContent, statErr := os.ReadFile(activeProfilePath)
	if errors.Is(statErr, os.ErrNotExist) || (statErr == nil && !bytes.Equal(profileContent, activeContent)) {
		if writeErr := atomicWriteFile(activeProfilePath, activeContent); writeErr != nil {
			return profileState{}, fmt.Errorf("initialize active profile: %w", writeErr)
		}
	} else if statErr != nil {
		return profileState{}, fmt.Errorf("inspect active profile: %w", statErr)
	}
	if state.Names[state.Active] == "" {
		state.Names[state.Active] = state.Active
	}
	if err := store.saveProfileStateUnlocked(state); err != nil {
		return profileState{}, err
	}
	return state, nil
}

func (store *configStore) profileReadPathUnlocked(profileID string, state profileState) (string, error) {
	if !validProfileID(profileID) {
		return "", errors.New("invalid configuration file")
	}
	path := store.profilePath(profileID)
	if profileID == state.Active {
		path = store.path
	}
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("configuration file %q: %w", profileID, os.ErrNotExist)
	}
	if err != nil {
		return "", fmt.Errorf("inspect configuration file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("configuration file is not a regular file")
	}
	return path, nil
}

func (store *configStore) listProfilesUnlocked(state profileState) ([]profileView, error) {
	entries, err := os.ReadDir(store.profilesDirectory())
	if err != nil {
		return nil, fmt.Errorf("list configuration files: %w", err)
	}
	profiles := make([]profileView, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || strings.ToLower(filepath.Ext(entry.Name())) != ".yml" {
			continue
		}
		profileID := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		if !validProfileID(profileID) {
			continue
		}
		path := store.profilePath(profileID)
		if profileID == state.Active {
			path = store.path
		}
		info, infoErr := os.Stat(path)
		if infoErr != nil || !info.Mode().IsRegular() {
			continue
		}
		name := strings.TrimSpace(state.Names[profileID])
		if name == "" {
			name = profileID
		}
		profiles = append(profiles, profileView{
			ID:       profileID,
			Name:     name,
			FileName: profileID + ".yml",
			Active:   profileID == state.Active,
			Modified: info.ModTime().Format(time.RFC3339),
		})
	}
	sort.Slice(profiles, func(left, right int) bool {
		if profiles[left].ID == "prometheus" {
			return true
		}
		if profiles[right].ID == "prometheus" {
			return false
		}
		return strings.ToLower(profiles[left].Name) < strings.ToLower(profiles[right].Name)
	})
	return profiles, nil
}

func (store *configStore) availableProfileIDUnlocked(name string) (string, error) {
	name, err := normalizeProfileName(name)
	if err != nil {
		return "", err
	}
	base := profileSlug(name)
	if base == "" {
		base = "config"
	}
	for suffix := 1; suffix < 10000; suffix++ {
		candidate := base
		if suffix > 1 {
			candidate = fmt.Sprintf("%s-%d", base, suffix)
		}
		if _, err := os.Stat(store.profilePath(candidate)); errors.Is(err, os.ErrNotExist) {
			return candidate, nil
		} else if err != nil {
			return "", fmt.Errorf("inspect configuration file: %w", err)
		}
	}
	return "", errors.New("too many configuration files with the same name")
}

func normalizeProfileName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("configuration file name is required")
	}
	if len([]rune(name)) > 80 {
		return "", errors.New("configuration file name is too long")
	}
	return name, nil
}

func (store *configStore) saveProfileStateUnlocked(state profileState) error {
	content, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode profile state: %w", err)
	}
	content = append(content, '\n')
	if err := atomicWriteFile(store.profileStatePath(), content); err != nil {
		return fmt.Errorf("write profile state: %w", err)
	}
	return nil
}

func validProfileID(profileID string) bool {
	if profileID == "" || len(profileID) > 96 {
		return false
	}
	for _, character := range profileID {
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') || character == '-' || character == '_' {
			continue
		}
		return false
	}
	return true
}

func profileSlug(name string) string {
	var builder strings.Builder
	lastSeparator := false
	for _, character := range strings.ToLower(strings.TrimSpace(name)) {
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') || character == '_' {
			builder.WriteRune(character)
			lastSeparator = false
			continue
		}
		if (character == '-' || character == ' ' || character == '.') && builder.Len() > 0 && !lastSeparator {
			builder.WriteByte('-')
			lastSeparator = true
		}
	}
	return strings.Trim(builder.String(), "-")
}

func (store *configStore) validateAndApplyProfileUnlocked(profileID string, next []byte, state profileState) (saveResult, profileState, error) {
	validationOutput, err := store.validate(next)
	if err != nil {
		return saveResult{Output: validationOutput}, state, err
	}
	currentActive, err := os.ReadFile(store.path)
	if err != nil {
		return saveResult{}, state, fmt.Errorf("read active config: %w", err)
	}
	backupPath, err := store.backupUnlocked(currentActive)
	if err != nil {
		return saveResult{}, state, err
	}
	profilePath := store.profilePath(profileID)
	if err := atomicWriteFile(profilePath, next); err != nil {
		return saveResult{}, state, fmt.Errorf("write profile: %w", err)
	}
	if err := atomicWriteFile(store.path, next); err != nil {
		return saveResult{}, state, fmt.Errorf("write active config: %w", err)
	}
	state.Active = profileID
	if err := store.saveProfileStateUnlocked(state); err != nil {
		return saveResult{}, state, err
	}
	_ = store.pruneBackupsUnlocked()
	return saveResult{BackupPath: backupPath, Output: validationOutput}, state, nil
}

func (store *configStore) validateAndWriteUnlocked(next, current []byte) (saveResult, error) {
	validationOutput, err := store.validate(next)
	if err != nil {
		return saveResult{Output: validationOutput}, err
	}
	backupPath, err := store.backupUnlocked(current)
	if err != nil {
		return saveResult{}, err
	}
	if err := atomicWriteFile(store.path, next); err != nil {
		return saveResult{}, fmt.Errorf("write config: %w", err)
	}
	_ = store.pruneBackupsUnlocked()
	return saveResult{BackupPath: backupPath, Output: validationOutput}, nil
}

func (store *configStore) validate(content []byte) (string, error) {
	if store.validateFunc != nil {
		return store.validateFunc(content)
	}
	temp, err := os.CreateTemp(filepath.Dir(store.path), ".prometheus-validate-*.yml")
	if err != nil {
		return "", fmt.Errorf("create validation file: %w", err)
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if _, err := temp.Write(content); err != nil {
		temp.Close()
		return "", fmt.Errorf("write validation file: %w", err)
	}
	if err := temp.Close(); err != nil {
		return "", fmt.Errorf("close validation file: %w", err)
	}

	command := exec.Command(store.promtoolPath, "check", "config", tempPath)
	output, err := command.CombinedOutput()
	trimmed := strings.TrimSpace(string(output))
	if err != nil {
		if trimmed == "" {
			trimmed = err.Error()
		}
		return trimmed, fmt.Errorf("promtool validation failed: %s", trimmed)
	}
	return trimmed, nil
}

func (store *configStore) backupUnlocked(content []byte) (string, error) {
	settings, err := store.loadBackupSettingsUnlocked()
	if err != nil {
		return "", err
	}
	if !settings.Enabled {
		return "", nil
	}
	directory := store.backupDirectory()
	if err := os.MkdirAll(directory, 0700); err != nil {
		return "", fmt.Errorf("create backup directory: %w", err)
	}
	name := fmt.Sprintf("prometheus.yml.%s", time.Now().Format("20060102-150405.000000000"))
	path := filepath.Join(directory, name)
	if err := os.WriteFile(path, content, 0600); err != nil {
		return "", fmt.Errorf("write backup: %w", err)
	}
	return path, nil
}

func (store *configStore) backupDirectory() string {
	return filepath.Join(filepath.Dir(store.path), ".prometheus-backups")
}

func (store *configStore) backupSettingsPath() string {
	return filepath.Join(filepath.Dir(store.path), ".prometheus-backup-settings.json")
}

func (store *configStore) loadBackupSettingsUnlocked() (backupSettings, error) {
	retention := store.backupLimit
	if retention < 1 {
		retention = 20
	}
	settings := backupSettings{Enabled: true, Retention: retention}
	content, err := os.ReadFile(store.backupSettingsPath())
	if errors.Is(err, os.ErrNotExist) {
		return settings, nil
	}
	if err != nil {
		return backupSettings{}, fmt.Errorf("read backup settings: %w", err)
	}
	var file struct {
		Enabled   *bool `json:"enabled"`
		Retention *int  `json:"retention"`
	}
	if err := json.Unmarshal(content, &file); err != nil {
		return backupSettings{}, fmt.Errorf("parse backup settings: %w", err)
	}
	if file.Enabled != nil {
		settings.Enabled = *file.Enabled
	}
	if file.Retention != nil {
		settings.Retention = *file.Retention
	}
	return normalizeBackupSettings(settings)
}

func normalizeBackupSettings(settings backupSettings) (backupSettings, error) {
	if settings.Retention < 1 || settings.Retention > 100 {
		return backupSettings{}, errors.New("backup retention must be between 1 and 100")
	}
	return settings, nil
}

func (store *configStore) saveBackupSettings(request backupSettingsRequest) (configView, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	settings, err := normalizeBackupSettings(backupSettings{Enabled: request.Enabled, Retention: request.Retention})
	if err != nil {
		return configView{}, err
	}
	content, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return configView{}, fmt.Errorf("encode backup settings: %w", err)
	}
	content = append(content, '\n')
	if err := os.WriteFile(store.backupSettingsPath(), content, 0600); err != nil {
		return configView{}, fmt.Errorf("write backup settings: %w", err)
	}
	if err := store.pruneBackupsUnlocked(); err != nil {
		return configView{}, fmt.Errorf("prune backups: %w", err)
	}
	return store.loadUnlocked()
}

func (store *configStore) deleteBackup(name string) (configView, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	if name == "" || strings.ContainsAny(name, `/\\`) || filepath.Base(name) != name || !strings.HasPrefix(name, "prometheus.yml.") {
		return configView{}, errors.New("invalid backup name")
	}
	path := filepath.Join(store.backupDirectory(), name)
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return configView{}, os.ErrNotExist
	}
	if err != nil {
		return configView{}, fmt.Errorf("inspect backup: %w", err)
	}
	if !info.Mode().IsRegular() {
		return configView{}, errors.New("backup is not a regular file")
	}
	if err := os.Remove(path); err != nil {
		return configView{}, fmt.Errorf("delete backup: %w", err)
	}
	return store.loadUnlocked()
}

func (store *configStore) listBackupsUnlocked() ([]backupView, error) {
	directory := store.backupDirectory()
	entries, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return []backupView{}, nil
	}
	if err != nil {
		return nil, err
	}
	backups := make([]backupView, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "prometheus.yml.") {
			continue
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			continue
		}
		backups = append(backups, backupView{
			Name:     entry.Name(),
			Modified: info.ModTime().Format(time.RFC3339),
			Size:     info.Size(),
		})
	}
	sort.Slice(backups, func(left, right int) bool { return backups[left].Name > backups[right].Name })
	return backups, nil
}

func (store *configStore) pruneBackupsUnlocked() error {
	settings, err := store.loadBackupSettingsUnlocked()
	if err != nil {
		return err
	}
	backups, err := store.listBackupsUnlocked()
	if err != nil || len(backups) <= settings.Retention {
		return err
	}
	for _, backup := range backups[settings.Retention:] {
		path := filepath.Join(store.backupDirectory(), backup.Name)
		if err := os.Remove(path); err != nil {
			return err
		}
	}
	return nil
}

func parseYAMLDocument(content []byte) (*yaml.Node, error) {
	var document yaml.Node
	decoder := yaml.NewDecoder(bytes.NewReader(content))
	if err := decoder.Decode(&document); err != nil {
		return nil, err
	}
	if len(document.Content) == 0 || document.Content[0].Kind != yaml.MappingNode {
		return nil, errors.New("top-level YAML value must be a mapping")
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, errors.New("only one YAML document is supported")
		}
		return nil, err
	}
	return &document, nil
}

func readGlobal(document *yaml.Node) globalView {
	global := mappingValue(document.Content[0], "global")
	if global == nil || global.Kind != yaml.MappingNode {
		return globalView{}
	}
	return globalView{
		ScrapeInterval:     scalarValue(mappingValue(global, "scrape_interval")),
		EvaluationInterval: scalarValue(mappingValue(global, "evaluation_interval")),
		ScrapeTimeout:      scalarValue(mappingValue(global, "scrape_timeout")),
	}
}

func readJobs(document *yaml.Node) []jobView {
	sequence := mappingValue(document.Content[0], "scrape_configs")
	if sequence == nil || sequence.Kind != yaml.SequenceNode {
		return []jobView{}
	}
	jobs := make([]jobView, 0, len(sequence.Content))
	for index, node := range sequence.Content {
		if node.Kind != yaml.MappingNode {
			continue
		}
		sourceIndex := index
		jobs = append(jobs, jobView{
			SourceIndex:    &sourceIndex,
			JobName:        scalarValue(mappingValue(node, "job_name")),
			MetricsPath:    scalarValue(mappingValue(node, "metrics_path")),
			Scheme:         scalarValue(mappingValue(node, "scheme")),
			ScrapeInterval: scalarValue(mappingValue(node, "scrape_interval")),
			ScrapeTimeout:  scalarValue(mappingValue(node, "scrape_timeout")),
			StaticConfigs:  readStaticConfigs(node),
			Advanced:       hasAdvancedJobFields(node),
		})
	}
	return jobs
}

func readStaticConfigs(job *yaml.Node) []staticConfigView {
	sequence := mappingValue(job, "static_configs")
	if sequence == nil || sequence.Kind != yaml.SequenceNode {
		return []staticConfigView{}
	}
	configs := make([]staticConfigView, 0, len(sequence.Content))
	for index, node := range sequence.Content {
		if node.Kind != yaml.MappingNode {
			continue
		}
		sourceIndex := index
		config := staticConfigView{SourceIndex: &sourceIndex, Labels: map[string]string{}}
		if targets := mappingValue(node, "targets"); targets != nil && targets.Kind == yaml.SequenceNode {
			for _, target := range targets.Content {
				config.Targets = append(config.Targets, scalarValue(target))
			}
		}
		if labels := mappingValue(node, "labels"); labels != nil && labels.Kind == yaml.MappingNode {
			for labelIndex := 0; labelIndex+1 < len(labels.Content); labelIndex += 2 {
				config.Labels[labels.Content[labelIndex].Value] = scalarValue(labels.Content[labelIndex+1])
			}
		}
		config.Advanced = hasUnknownMappingKeys(node, map[string]bool{"targets": true, "labels": true})
		configs = append(configs, config)
	}
	aliasRules := readDeviceAliasRules(job)
	for configIndex := range configs {
		for _, target := range configs[configIndex].Targets {
			configs[configIndex].Devices = append(configs[configIndex].Devices, deviceView{
				Address: target,
				Alias:   deviceAlias(target, aliasRules),
			})
		}
	}
	return configs
}

func hasAdvancedJobFields(job *yaml.Node) bool {
	if hasUnknownMappingKeys(job, map[string]bool{
		"job_name": true, "metrics_path": true, "scheme": true,
		"scrape_interval": true, "scrape_timeout": true, "static_configs": true,
		"relabel_configs": true,
	}) {
		return true
	}
	sequence := mappingValue(job, "relabel_configs")
	if sequence == nil {
		return false
	}
	if sequence.Kind != yaml.SequenceNode {
		return true
	}
	for _, rule := range sequence.Content {
		if _, _, ok := managedDeviceAliasRule(rule); !ok {
			return true
		}
	}
	return false
}

func readDeviceAliasRules(job *yaml.Node) []deviceAliasRule {
	sequence := mappingValue(job, "relabel_configs")
	if sequence == nil || sequence.Kind != yaml.SequenceNode {
		return nil
	}
	rules := make([]deviceAliasRule, 0, len(sequence.Content))
	for _, node := range sequence.Content {
		pattern, replacement, ok := managedDeviceAliasRule(node)
		if ok {
			rules = append(rules, deviceAliasRule{Pattern: pattern, Replacement: replacement})
		}
	}
	return rules
}

func managedDeviceAliasRule(node *yaml.Node) (string, string, bool) {
	if node == nil || node.Kind != yaml.MappingNode || hasUnknownMappingKeys(node, map[string]bool{
		"source_labels": true, "regex": true, "target_label": true,
		"replacement": true, "action": true,
	}) {
		return "", "", false
	}
	sourceLabels := mappingValue(node, "source_labels")
	if sourceLabels == nil || sourceLabels.Kind != yaml.SequenceNode || len(sourceLabels.Content) != 1 || scalarValue(sourceLabels.Content[0]) != "__address__" {
		return "", "", false
	}
	if scalarValue(mappingValue(node, "target_label")) != "alias" {
		return "", "", false
	}
	action := scalarValue(mappingValue(node, "action"))
	if action != "" && action != "replace" {
		return "", "", false
	}
	pattern := scalarValue(mappingValue(node, "regex"))
	if pattern == "" {
		pattern = "(.*)"
	}
	replacement := scalarValue(mappingValue(node, "replacement"))
	if replacement == "" {
		replacement = "$1"
	}
	if _, err := regexp.Compile("^(?:" + pattern + ")$"); err != nil {
		return "", "", false
	}
	return pattern, replacement, true
}

func deviceAlias(address string, rules []deviceAliasRule) string {
	alias := ""
	for _, rule := range rules {
		matcher, err := regexp.Compile("^(?:" + rule.Pattern + ")$")
		if err == nil && matcher.MatchString(address) {
			alias = matcher.ReplaceAllString(address, rule.Replacement)
		}
	}
	return alias
}

func hasUnknownMappingKeys(mapping *yaml.Node, known map[string]bool) bool {
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		if !known[mapping.Content[index].Value] {
			return true
		}
	}
	return false
}

func patchGlobal(document *yaml.Node, view globalView) {
	global := ensureMapping(document.Content[0], "global")
	setOptionalScalar(global, "scrape_interval", view.ScrapeInterval)
	setOptionalScalar(global, "evaluation_interval", view.EvaluationInterval)
	setOptionalScalar(global, "scrape_timeout", view.ScrapeTimeout)
}

func patchJobs(document *yaml.Node, jobs []jobView) error {
	root := document.Content[0]
	sequence := mappingValue(root, "scrape_configs")
	if sequence == nil || sequence.Kind != yaml.SequenceNode {
		sequence = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		setMappingValue(root, "scrape_configs", sequence)
	}
	original := append([]*yaml.Node(nil), sequence.Content...)
	usedIndexes := map[int]bool{}
	jobNames := map[string]bool{}
	next := make([]*yaml.Node, 0, len(jobs))
	for _, job := range jobs {
		job.JobName = strings.TrimSpace(job.JobName)
		if job.JobName == "" {
			return errors.New("job name cannot be empty")
		}
		if jobNames[job.JobName] {
			return fmt.Errorf("duplicate job name %q", job.JobName)
		}
		jobNames[job.JobName] = true

		var node *yaml.Node
		if job.SourceIndex != nil && *job.SourceIndex >= 0 && *job.SourceIndex < len(original) && !usedIndexes[*job.SourceIndex] {
			node = original[*job.SourceIndex]
			usedIndexes[*job.SourceIndex] = true
		}
		if node == nil || node.Kind != yaml.MappingNode {
			node = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		}
		patchJob(node, job)
		next = append(next, node)
	}
	sequence.Content = next
	return nil
}

func patchJob(node *yaml.Node, job jobView) {
	setOptionalScalar(node, "job_name", job.JobName)
	setOptionalScalar(node, "metrics_path", job.MetricsPath)
	setOptionalScalar(node, "scheme", job.Scheme)
	setOptionalScalar(node, "scrape_interval", job.ScrapeInterval)
	setOptionalScalar(node, "scrape_timeout", job.ScrapeTimeout)

	if len(job.StaticConfigs) == 0 {
		removeMappingValue(node, "static_configs")
		patchDeviceAliases(node, nil)
		return
	}
	existing := mappingValue(node, "static_configs")
	original := []*yaml.Node{}
	if existing != nil && existing.Kind == yaml.SequenceNode {
		original = append(original, existing.Content...)
	}
	sequence := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	usedIndexes := map[int]bool{}
	for _, config := range job.StaticConfigs {
		var mapping *yaml.Node
		if config.SourceIndex != nil && *config.SourceIndex >= 0 && *config.SourceIndex < len(original) && !usedIndexes[*config.SourceIndex] {
			mapping = original[*config.SourceIndex]
			usedIndexes[*config.SourceIndex] = true
		}
		if mapping == nil || mapping.Kind != yaml.MappingNode {
			mapping = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		}
		patchStaticConfig(mapping, config)
		sequence.Content = append(sequence.Content, mapping)
	}
	setMappingValue(node, "static_configs", sequence)
	patchDeviceAliases(node, job.StaticConfigs)
}

func patchStaticConfig(mapping *yaml.Node, config staticConfigView) {
	targets := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Style: yaml.FlowStyle}
	addresses := config.Targets
	if config.Devices != nil {
		addresses = make([]string, 0, len(config.Devices))
		for _, device := range config.Devices {
			addresses = append(addresses, device.Address)
		}
	}
	for _, target := range addresses {
		if target = strings.TrimSpace(target); target != "" {
			targets.Content = append(targets.Content, scalarNode(target))
		}
	}
	setMappingValue(mapping, "targets", targets)
	if len(config.Labels) == 0 {
		removeMappingValue(mapping, "labels")
		return
	}
	labels := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	keys := make([]string, 0, len(config.Labels))
	for key := range config.Labels {
		if strings.TrimSpace(key) != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		labels.Content = append(labels.Content, scalarNode(strings.TrimSpace(key)), scalarNode(config.Labels[key]))
	}
	if len(labels.Content) == 0 {
		removeMappingValue(mapping, "labels")
		return
	}
	setMappingValue(mapping, "labels", labels)
}

func patchDeviceAliases(job *yaml.Node, configs []staticConfigView) {
	existing := mappingValue(job, "relabel_configs")
	sequence := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	if existing != nil && existing.Kind == yaml.SequenceNode {
		for _, rule := range existing.Content {
			if _, _, managed := managedDeviceAliasRule(rule); !managed {
				sequence.Content = append(sequence.Content, rule)
			}
		}
	}
	seen := map[string]bool{}
	for _, config := range configs {
		for _, device := range config.Devices {
			address := strings.TrimSpace(device.Address)
			alias := strings.TrimSpace(device.Alias)
			if address == "" || alias == "" || seen[address] {
				continue
			}
			seen[address] = true
			rule := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			sourceLabels := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Style: yaml.FlowStyle}
			sourceLabels.Content = append(sourceLabels.Content, scalarNode("__address__"))
			setMappingValue(rule, "source_labels", sourceLabels)
			setMappingValue(rule, "regex", scalarNode(regexp.QuoteMeta(address)))
			setMappingValue(rule, "target_label", scalarNode("alias"))
			setMappingValue(rule, "replacement", scalarNode(alias))
			sequence.Content = append(sequence.Content, rule)
		}
	}
	if len(sequence.Content) == 0 {
		removeMappingValue(job, "relabel_configs")
		return
	}
	setMappingValue(job, "relabel_configs", sequence)
}

func mappingValue(mapping *yaml.Node, key string) *yaml.Node {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil
	}
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		if mapping.Content[index].Value == key {
			return mapping.Content[index+1]
		}
	}
	return nil
}

func ensureMapping(mapping *yaml.Node, key string) *yaml.Node {
	value := mappingValue(mapping, key)
	if value != nil && value.Kind == yaml.MappingNode {
		return value
	}
	value = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	setMappingValue(mapping, key, value)
	return value
}

func setOptionalScalar(mapping *yaml.Node, key, value string) {
	if value = strings.TrimSpace(value); value == "" {
		removeMappingValue(mapping, key)
		return
	}
	setMappingValue(mapping, key, scalarNode(value))
}

func setMappingValue(mapping *yaml.Node, key string, value *yaml.Node) {
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		if mapping.Content[index].Value == key {
			mapping.Content[index+1] = value
			return
		}
	}
	mapping.Content = append(mapping.Content, scalarNode(key), value)
}

func removeMappingValue(mapping *yaml.Node, key string) {
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		if mapping.Content[index].Value == key {
			mapping.Content = append(mapping.Content[:index], mapping.Content[index+2:]...)
			return
		}
	}
}

func scalarNode(value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
}

func scalarValue(node *yaml.Node) string {
	if node == nil {
		return ""
	}
	return node.Value
}

func checksum(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func atomicWriteFile(path string, content []byte) error {
	info, err := os.Stat(path)
	mode := os.FileMode(0600)
	if err == nil {
		mode = info.Mode()
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".prometheus-write-*.yml")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if _, err := temp.Write(content); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Chmod(mode); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, path); err == nil {
		return nil
	} else if runtime.GOOS != "windows" {
		return err
	}
	if err := os.Remove(path); err != nil {
		return err
	}
	return os.Rename(tempPath, path)
}
