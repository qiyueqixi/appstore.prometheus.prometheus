package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestMigrationInspectorDetectsOriginalData(t *testing.T) {
	source := t.TempDir()
	target := t.TempDir()
	writeMigrationFixture(t, source)

	status := (&migrationInspector{enabled: true, sourceShare: source, targetShare: target, command: "migrate"}).inspect()
	if !status.Enabled || !status.SourceDetected || !status.MigrationRequired || status.Migrated {
		t.Fatalf("unexpected migration status: %+v", status)
	}
}

func TestMigrationInspectorHidesCompletedMigration(t *testing.T) {
	source := t.TempDir()
	target := t.TempDir()
	writeMigrationFixture(t, source)
	if err := os.WriteFile(filepath.Join(target, ".migration-from-prometheus.prometheus"), []byte("done\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	status := (&migrationInspector{enabled: true, sourceShare: source, targetShare: target, command: "migrate"}).inspect()
	if !status.Enabled || !status.SourceDetected || status.MigrationRequired || !status.Migrated {
		t.Fatalf("unexpected migration status: %+v", status)
	}
}

func TestHandleMigration(t *testing.T) {
	app := &application{migration: &migrationInspector{enabled: true, sourceShare: t.TempDir(), targetShare: t.TempDir(), command: "migrate"}}
	request := httptest.NewRequest(http.MethodGet, "http://manager.test/api/migration", nil)
	response := httptest.NewRecorder()

	app.handleMigration(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("unexpected status code: %d", response.Code)
	}
	var payload apiResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if !payload.OK {
		t.Fatal("expected successful response")
	}
}

func TestMigrationInspectorCanBeDisabledForInPlaceUpgrade(t *testing.T) {
	source := t.TempDir()
	writeMigrationFixture(t, source)

	status := (&migrationInspector{sourceShare: source, targetShare: source, command: "migrate"}).inspect()
	if status.Enabled || status.SourceDetected || status.MigrationRequired || status.Migrated {
		t.Fatalf("disabled migration should not inspect the original data directory: %+v", status)
	}
	if status.SourceDirectory != "" || status.TargetDirectory != "" || status.Command != "" {
		t.Fatalf("disabled migration should not expose migration details: %+v", status)
	}
}

func writeMigrationFixture(t *testing.T, source string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(source, "prometheus.yml"), []byte("global: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(source, "data"), 0o700); err != nil {
		t.Fatal(err)
	}
}
