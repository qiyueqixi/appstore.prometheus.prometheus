package main

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type migrationInspector struct {
	enabled     bool
	sourceShare string
	targetShare string
	command     string
}

type migrationStatus struct {
	Enabled           bool   `json:"enabled"`
	SourceDetected    bool   `json:"sourceDetected"`
	Migrated          bool   `json:"migrated"`
	MigrationRequired bool   `json:"migrationRequired"`
	SourceDirectory   string `json:"sourceDirectory"`
	TargetDirectory   string `json:"targetDirectory"`
	Command           string `json:"command"`
	DetectionError    string `json:"detectionError,omitempty"`
}

func (app *application) handleMigration(response http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		methodNotAllowed(response, http.MethodGet)
		return
	}
	writeJSON(response, http.StatusOK, apiResponse{OK: true, Data: app.migration.inspect()})
}

func (inspector *migrationInspector) inspect() migrationStatus {
	status := migrationStatus{
		Enabled: inspector.enabled,
	}
	if !inspector.enabled {
		return status
	}
	status.SourceDirectory = inspector.sourceShare
	status.TargetDirectory = inspector.targetShare
	status.Command = inspector.command

	configDetected, configErr := regularFileExists(filepath.Join(inspector.sourceShare, "prometheus.yml"))
	dataDetected, dataErr := directoryExists(filepath.Join(inspector.sourceShare, "data"))
	status.SourceDetected = configDetected && dataDetected

	markerDetected, markerErr := regularFileExists(filepath.Join(inspector.targetShare, ".migration-from-prometheus.prometheus"))
	status.Migrated = markerDetected
	status.MigrationRequired = status.SourceDetected && !status.Migrated

	var detectionErrors []string
	for _, err := range []error{configErr, dataErr, markerErr} {
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			detectionErrors = append(detectionErrors, err.Error())
		}
	}
	status.DetectionError = strings.Join(detectionErrors, "; ")
	return status
}

func regularFileExists(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	return info.Mode().IsRegular(), nil
}

func directoryExists(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	return info.IsDir(), nil
}
