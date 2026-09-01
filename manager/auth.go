package main

import (
	"errors"
	"net/http"
	"os"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gopkg.in/yaml.v3"
)

type authProvider struct {
	path     string
	mu       sync.RWMutex
	modified time.Time
	size     int64
	users    map[string]string
}

type webConfig struct {
	BasicAuthUsers map[string]string `yaml:"basic_auth_users"`
}

func (provider *authProvider) authenticate(request *http.Request) bool {
	users, err := provider.loadUsers()
	if err != nil || len(users) == 0 {
		return err == nil
	}
	username, password, ok := request.BasicAuth()
	if !ok {
		return false
	}
	hash, ok := users[username]
	if !ok {
		bcrypt.CompareHashAndPassword([]byte("$2y$10$7EqJtq98hPqEX7fNZaFWoO5HnE4zHM4W8sTjEvvVYstRClJtQGv1a"), []byte(password))
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

func (provider *authProvider) loadUsers() (map[string]string, error) {
	if provider.path == "" {
		return map[string]string{}, nil
	}
	info, err := os.Stat(provider.path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}

	provider.mu.RLock()
	if info.ModTime().Equal(provider.modified) && info.Size() == provider.size {
		users := provider.users
		provider.mu.RUnlock()
		return users, nil
	}
	provider.mu.RUnlock()

	raw, err := os.ReadFile(provider.path)
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		provider.store(info, map[string]string{})
		return map[string]string{}, nil
	}
	var config webConfig
	if err := yaml.Unmarshal(raw, &config); err != nil {
		return nil, err
	}
	if config.BasicAuthUsers == nil {
		config.BasicAuthUsers = map[string]string{}
	}
	provider.store(info, config.BasicAuthUsers)
	return config.BasicAuthUsers, nil
}

func (provider *authProvider) store(info os.FileInfo, users map[string]string) {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	provider.modified = info.ModTime()
	provider.size = info.Size()
	provider.users = users
}

func basicAuthMiddleware(provider *authProvider, next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/healthz" {
			next.ServeHTTP(response, request)
			return
		}
		if !provider.authenticate(request) {
			response.Header().Set("WWW-Authenticate", `Basic realm="Prometheus Control", charset="UTF-8"`)
			http.Error(response, "authentication required", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(response, request)
	})
}

func hashPassword(password []byte) (string, error) {
	hash, err := bcrypt.GenerateFromPassword(password, bcrypt.DefaultCost)
	return string(hash), err
}
