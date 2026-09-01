package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestPrometheusWebProxyStripsRoutePrefix(t *testing.T) {
	var receivedPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		receivedPath = request.URL.Path
		response.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(response, "ok")
	}))
	defer upstream.Close()

	target, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	proxy := newPrometheusProxy(target, true)
	request := httptest.NewRequest(http.MethodGet, "http://manager.test/prometheus/api/v1/query?query=up", nil)
	response := httptest.NewRecorder()
	proxy.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("unexpected proxy status: %d", response.Code)
	}
	if receivedPath != "/api/v1/query" {
		t.Fatalf("unexpected upstream path: %q", receivedPath)
	}
}

func TestCompatibilityProxyKeepsRootPath(t *testing.T) {
	var receivedPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		receivedPath = request.URL.Path
		response.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	target, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	proxy := newPrometheusProxy(target, false)
	request := httptest.NewRequest(http.MethodGet, "http://manager.test/api/v1/query?query=up", nil)
	response := httptest.NewRecorder()
	proxy.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("unexpected proxy status: %d", response.Code)
	}
	if receivedPath != "/api/v1/query" {
		t.Fatalf("unexpected upstream path: %q", receivedPath)
	}
}
