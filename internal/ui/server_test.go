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
	for _, want := range []string{"Jobs", "History", "Settings"} {
		if !strings.Contains(response.Body.String(), want) {
			t.Fatalf("body does not contain %q view", want)
		}
	}
}

func TestServerExecutesCommand(t *testing.T) {
	server, err := NewServer()
	if err != nil {
		t.Fatalf("NewServer returned error: %v", err)
	}

	response := executeRequest(t, server, `{"sessionId":"test-a","command":"echo hello-ui"}`)
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

	response := executeRequest(t, server, `{"sessionId":"test-a","command":"   "}`)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", response.Code)
	}
	if !strings.Contains(response.Body.String(), "command is required") {
		t.Fatalf("body = %q, want validation error", response.Body.String())
	}
}

func TestServerKeepsExitedSessionIsolated(t *testing.T) {
	server, err := NewServer()
	if err != nil {
		t.Fatalf("NewServer returned error: %v", err)
	}

	firstExit := executeRequest(t, server, `{"sessionId":"closed-tab","command":"exit"}`)
	if firstExit.Code != http.StatusOK {
		t.Fatalf("exit status = %d, want 200; body = %s", firstExit.Code, firstExit.Body.String())
	}
	sameSession := executeRequest(t, server, `{"sessionId":"closed-tab","command":"echo after-exit"}`)
	if sameSession.Code != http.StatusBadRequest {
		t.Fatalf("same session status = %d, want 400", sameSession.Code)
	}
	newSession := executeRequest(t, server, `{"sessionId":"fresh-tab","command":"echo fresh"}`)
	if newSession.Code != http.StatusOK {
		t.Fatalf("fresh session status = %d, want 200; body = %s", newSession.Code, newSession.Body.String())
	}
	if !strings.Contains(newSession.Body.String(), "fresh") {
		t.Fatalf("fresh session body = %q, want fresh output", newSession.Body.String())
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
