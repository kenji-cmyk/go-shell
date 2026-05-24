package ui

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestServerServesIndex(t *testing.T) {
	server, err := NewServer()
	if err != nil {
		t.Fatalf("NewServer returned error: %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	if !strings.Contains(response.Body.String(), "Go Shell UI") {
		t.Fatalf("body does not contain UI title")
	}
}

func TestServerExecutesCommand(t *testing.T) {
	server, err := NewServer()
	if err != nil {
		t.Fatalf("NewServer returned error: %v", err)
	}

	response := executeRequest(t, server, `{"command":"echo hello-ui"}`)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", response.Code, response.Body.String())
	}

	var payload ExecuteResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if !payload.OK || strings.TrimSpace(payload.Stdout) != "hello-ui" {
		t.Fatalf("payload = %#v, want successful echo output", payload)
	}
}

func TestServerRejectsEmptyCommand(t *testing.T) {
	server, err := NewServer()
	if err != nil {
		t.Fatalf("NewServer returned error: %v", err)
	}

	response := executeRequest(t, server, `{"command":"   "}`)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", response.Code)
	}
	if !strings.Contains(response.Body.String(), "command is required") {
		t.Fatalf("body = %q, want validation error", response.Body.String())
	}
}

func executeRequest(t *testing.T, server *Server, body string) *httptest.ResponseRecorder {
	t.Helper()

	request := httptest.NewRequest(http.MethodPost, "/api/execute", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	return response
}
