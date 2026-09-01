package main

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

//go:embed web/*
var embeddedWeb embed.FS

type application struct {
	store       *configStore
	migration   *migrationInspector
	reloadURL   string
	readyURL    string
	httpClient  *http.Client
	staticFiles http.Handler
}

type apiResponse struct {
	OK          bool        `json:"ok"`
	Message     string      `json:"message,omitempty"`
	Error       string      `json:"error,omitempty"`
	Details     string      `json:"details,omitempty"`
	ReloadOK    bool        `json:"reloadOk,omitempty"`
	ReloadError string      `json:"reloadError,omitempty"`
	Backup      string      `json:"backup,omitempty"`
	Data        interface{} `json:"data,omitempty"`
}

func main() {
	if len(os.Args) == 2 && os.Args[1] == "hash-password" {
		password, err := io.ReadAll(io.LimitReader(os.Stdin, 4096))
		if err != nil {
			log.Fatal(err)
		}
		hash, err := hashPassword(bytesTrimSpace(password))
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(hash)
		return
	}
	listenAddress := environment("LISTEN_ADDR", ":19090")
	configPath := environment("PROMETHEUS_CONFIG", "/var/apps/prometheus.control/shares/prometheus-control/prometheus.yml")
	promtoolPath := environment("PROMTOOL_BIN", filepath.Join(filepath.Dir(os.Args[0]), "promtool"))
	webConfigPath := environment("PROMETHEUS_WEB_CONFIG", "")
	targetAddress := environment("PROMETHEUS_TARGET", "http://127.0.0.1:19091")
	reloadURL := environment("PROMETHEUS_RELOAD_URL", targetAddress+"/-/reload")
	readyURL := environment("PROMETHEUS_READY_URL", targetAddress+"/-/ready")
	targetShare := environment("PROMETHEUS_SHARE", filepath.Dir(configPath))
	sourceShare := environment("ORIGINAL_PROMETHEUS_SHARE", "/var/apps/prometheus.prometheus/shares/prometheus/prometheus")
	migrationCommand := environment("PROMETHEUS_MIGRATION_COMMAND", "sudo /var/apps/prometheus.control/target/cmd/migrate_from_original")
	migrationEnabled := !strings.EqualFold(environment("PROMETHEUS_MIGRATION_ENABLED", "true"), "false")

	target, err := url.Parse(targetAddress)
	if err != nil {
		log.Fatalf("invalid PROMETHEUS_TARGET: %v", err)
	}
	webRoot, err := fs.Sub(embeddedWeb, "web")
	if err != nil {
		log.Fatalf("load embedded web files: %v", err)
	}

	proxy := newPrometheusProxy(target, true)
	compatibilityProxy := newPrometheusProxy(target, false)

	app := &application{
		store: &configStore{
			path:                    configPath,
			promtoolPath:            promtoolPath,
			backupLimit:             20,
			defaultPrometheusTarget: target.Host,
		},
		migration: &migrationInspector{
			enabled:     migrationEnabled,
			sourceShare: sourceShare,
			targetShare: targetShare,
			command:     migrationCommand,
		},
		reloadURL:   reloadURL,
		readyURL:    readyURL,
		httpClient:  &http.Client{Timeout: 5 * time.Second},
		staticFiles: http.FileServer(http.FS(webRoot)),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", app.handleHealth)
	mux.HandleFunc("/api/status", app.handleStatus)
	mux.HandleFunc("/api/migration", app.handleMigration)
	mux.HandleFunc("/api/config", app.handleConfig)
	mux.HandleFunc("/api/config/profiles", app.handleConfigProfiles)
	mux.HandleFunc("/api/config/profiles/", app.handleConfigProfile)
	mux.HandleFunc("/api/config/basic", app.handleBasicConfig)
	mux.HandleFunc("/api/config/raw", app.handleRawConfig)
	mux.HandleFunc("/api/backups/settings", app.handleBackupSettings)
	mux.HandleFunc("/api/backups/", app.handleBackup)
	mux.HandleFunc("/api/backups", app.handleBackups)
	mux.HandleFunc("/prometheus", func(response http.ResponseWriter, request *http.Request) {
		http.Redirect(response, request, "/prometheus/", http.StatusTemporaryRedirect)
	})
	mux.Handle("/prometheus/", proxy)
	mux.Handle("/api/v1", compatibilityProxy)
	mux.Handle("/api/v1/", compatibilityProxy)
	mux.Handle("/-/", compatibilityProxy)
	mux.Handle("/federate", compatibilityProxy)
	mux.Handle("/metrics", compatibilityProxy)
	mux.HandleFunc("/", app.handleStatic)

	handler := securityHeaders(basicAuthMiddleware(&authProvider{path: webConfigPath}, mux))
	server := &http.Server{
		Addr:              listenAddress,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	log.Printf("Prometheus Control listening on %s", listenAddress)
	if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func (app *application) handleHealth(response http.ResponseWriter, _ *http.Request) {
	writeJSON(response, http.StatusOK, apiResponse{OK: true, Message: "Prometheus Control is ready"})
}

func (app *application) handleStatus(response http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		methodNotAllowed(response, http.MethodGet)
		return
	}
	upstreamResponse, err := app.httpClient.Get(app.readyURL)
	if err != nil {
		writeJSON(response, http.StatusOK, apiResponse{OK: true, Data: map[string]interface{}{"ready": false, "reason": err.Error()}})
		return
	}
	defer upstreamResponse.Body.Close()
	writeJSON(response, http.StatusOK, apiResponse{OK: true, Data: map[string]interface{}{
		"ready":  upstreamResponse.StatusCode == http.StatusOK,
		"status": upstreamResponse.StatusCode,
	}})
}

func (app *application) handleConfig(response http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		methodNotAllowed(response, http.MethodGet)
		return
	}
	view, err := app.store.loadProfile(request.URL.Query().Get("profile"))
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, os.ErrNotExist) {
			status = http.StatusNotFound
		}
		writeError(response, status, err)
		return
	}
	writeJSON(response, http.StatusOK, apiResponse{OK: true, Data: view})
}

func (app *application) handleConfigProfiles(response http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		methodNotAllowed(response, http.MethodPost)
		return
	}
	var payload createProfileRequest
	if err := decodeJSON(response, request, &payload); err != nil {
		writeError(response, http.StatusBadRequest, err)
		return
	}
	view, err := app.store.createProfile(payload)
	if err != nil {
		writeError(response, http.StatusBadRequest, err)
		return
	}
	writeJSON(response, http.StatusCreated, apiResponse{OK: true, Message: "Configuration file created", Data: view})
}

func (app *application) handleConfigProfile(response http.ResponseWriter, request *http.Request) {
	profileID := strings.TrimPrefix(request.URL.Path, "/api/config/profiles/")
	profileID, err := url.PathUnescape(profileID)
	if err != nil || !validProfileID(profileID) {
		writeError(response, http.StatusBadRequest, errors.New("invalid configuration file"))
		return
	}

	switch request.Method {
	case http.MethodPut:
		var payload renameProfileRequest
		if err := decodeJSON(response, request, &payload); err != nil {
			writeError(response, http.StatusBadRequest, err)
			return
		}
		view, err := app.store.renameProfile(profileID, payload)
		if err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, os.ErrNotExist) {
				status = http.StatusNotFound
			}
			writeError(response, status, err)
			return
		}
		writeJSON(response, http.StatusOK, apiResponse{OK: true, Message: "Configuration file renamed", Data: view})
	case http.MethodDelete:
		view, err := app.store.deleteProfile(profileID)
		if err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, os.ErrNotExist) {
				status = http.StatusNotFound
			}
			writeError(response, status, err)
			return
		}
		writeJSON(response, http.StatusOK, apiResponse{OK: true, Message: "Configuration file deleted", Data: view})
	default:
		methodNotAllowed(response, http.MethodPut, http.MethodDelete)
	}
}

func (app *application) handleBasicConfig(response http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPut {
		methodNotAllowed(response, http.MethodPut)
		return
	}
	var payload basicConfigRequest
	if err := decodeJSON(response, request, &payload); err != nil {
		writeError(response, http.StatusBadRequest, err)
		return
	}
	result, view, err := app.store.saveBasic(payload)
	app.finishSave(response, result, view, err)
}

func (app *application) handleRawConfig(response http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPut {
		methodNotAllowed(response, http.MethodPut)
		return
	}
	var payload rawConfigRequest
	if err := decodeJSON(response, request, &payload); err != nil {
		writeError(response, http.StatusBadRequest, err)
		return
	}
	result, view, err := app.store.saveRaw(payload)
	app.finishSave(response, result, view, err)
}

func (app *application) handleBackupSettings(response http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPut {
		methodNotAllowed(response, http.MethodPut)
		return
	}
	var payload backupSettingsRequest
	if err := decodeJSON(response, request, &payload); err != nil {
		writeError(response, http.StatusBadRequest, err)
		return
	}
	view, err := app.store.saveBackupSettings(payload)
	if err != nil {
		writeError(response, http.StatusBadRequest, err)
		return
	}
	writeJSON(response, http.StatusOK, apiResponse{OK: true, Message: "Backup settings saved", Data: view})
}

func (app *application) handleBackups(response http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		methodNotAllowed(response, http.MethodGet)
		return
	}
	view, err := app.store.load()
	if err != nil {
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	writeJSON(response, http.StatusOK, apiResponse{OK: true, Data: map[string]interface{}{
		"backups":         view.Backups,
		"settings":        view.BackupSettings,
		"backupDirectory": view.BackupDirectory,
	}})
}

func (app *application) handleBackup(response http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodDelete {
		methodNotAllowed(response, http.MethodDelete)
		return
	}
	name := strings.TrimPrefix(request.URL.Path, "/api/backups/")
	name, err := url.PathUnescape(name)
	if err != nil {
		writeError(response, http.StatusBadRequest, errors.New("invalid backup name"))
		return
	}
	view, err := app.store.deleteBackup(name)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, os.ErrNotExist) {
			status = http.StatusNotFound
		}
		writeError(response, status, err)
		return
	}
	writeJSON(response, http.StatusOK, apiResponse{OK: true, Message: "Backup deleted", Data: view})
}

func (app *application) finishSave(response http.ResponseWriter, result saveResult, view configView, err error) {
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, errConfigChanged) {
			status = http.StatusConflict
		}
		writeJSON(response, status, apiResponse{OK: false, Error: err.Error(), Details: result.Output})
		return
	}

	reloadErr := app.reloadPrometheus()
	payload := apiResponse{
		OK:       true,
		Message:  "Configuration saved and validated",
		Backup:   filepath.Base(result.BackupPath),
		ReloadOK: reloadErr == nil,
		Data:     view,
	}
	if reloadErr != nil {
		payload.ReloadError = reloadErr.Error()
		payload.Message = "Configuration saved, but reload failed"
	}
	writeJSON(response, http.StatusOK, payload)
}

func (app *application) reloadPrometheus() error {
	request, err := http.NewRequest(http.MethodPost, app.reloadURL, nil)
	if err != nil {
		return err
	}
	response, err := app.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("Prometheus reload returned HTTP %d", response.StatusCode)
	}
	return nil
}

func (app *application) handleStatic(response http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		methodNotAllowed(response, http.MethodGet, http.MethodHead)
		return
	}
	switch request.URL.Path {
	case "/", "/index.html", "/app.js", "/styles.css", "/favicon.svg":
		app.staticFiles.ServeHTTP(response, request)
	default:
		destination := "/prometheus" + request.URL.RequestURI()
		http.Redirect(response, request, destination, http.StatusTemporaryRedirect)
	}
}

func newPrometheusProxy(target *url.URL, stripRoutePrefix bool) *httputil.ReverseProxy {
	proxy := httputil.NewSingleHostReverseProxy(target)
	director := proxy.Director
	proxy.Director = func(request *http.Request) {
		if stripRoutePrefix {
			request.URL.Path = strings.TrimPrefix(request.URL.Path, "/prometheus")
			if request.URL.Path == "" {
				request.URL.Path = "/"
			}
			if request.URL.RawPath != "" {
				request.URL.RawPath = strings.TrimPrefix(request.URL.RawPath, "/prometheus")
				if request.URL.RawPath == "" {
					request.URL.RawPath = "/"
				}
			}
		}
		director(request)
	}
	proxy.ErrorHandler = func(response http.ResponseWriter, request *http.Request, proxyErr error) {
		log.Printf("prometheus proxy error: %v", proxyErr)
		http.Error(response, "Prometheus is not ready", http.StatusBadGateway)
	}
	proxy.ModifyResponse = func(response *http.Response) error {
		response.Header.Del("X-Frame-Options")
		return nil
	}
	return proxy
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("X-Content-Type-Options", "nosniff")
		response.Header().Set("Referrer-Policy", "same-origin")
		response.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		if !strings.HasPrefix(request.URL.Path, "/prometheus/") {
			response.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; script-src 'self'; img-src 'self' data:; frame-src 'self'; connect-src 'self'")
		}
		next.ServeHTTP(response, request)
	})
}

func decodeJSON(response http.ResponseWriter, request *http.Request, destination interface{}) error {
	defer request.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(response, request.Body, 2<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	return nil
}

func writeError(response http.ResponseWriter, status int, err error) {
	writeJSON(response, status, apiResponse{OK: false, Error: err.Error()})
}

func writeJSON(response http.ResponseWriter, status int, payload apiResponse) {
	response.Header().Set("Content-Type", "application/json; charset=utf-8")
	response.WriteHeader(status)
	if err := json.NewEncoder(response).Encode(payload); err != nil {
		log.Printf("write response: %v", err)
	}
}

func methodNotAllowed(response http.ResponseWriter, allowed ...string) {
	response.Header().Set("Allow", strings.Join(allowed, ", "))
	writeJSON(response, http.StatusMethodNotAllowed, apiResponse{OK: false, Error: "method not allowed"})
}

func environment(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func bytesTrimSpace(value []byte) []byte {
	return []byte(strings.TrimSpace(string(value)))
}
