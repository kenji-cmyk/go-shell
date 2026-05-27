package ui

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strconv"
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

func TestStaticUIIncludesTerminalProtocolHandling(t *testing.T) {
	data, err := staticFiles.ReadFile("static/app.js")
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	source := string(data)
	for _, want := range []string{"renderTerminalText", "parseSGR", "ansi-fg-", "?1049"} {
		if !strings.Contains(source, want) {
			t.Fatalf("app.js does not contain %q terminal protocol handling", want)
		}
	}
	styles, err := staticFiles.ReadFile("static/app.css")
	if err != nil {
		t.Fatalf("ReadFile CSS returned error: %v", err)
	}
	if !strings.Contains(string(styles), "ansi-fg-32") {
		t.Fatalf("app.css does not contain ANSI color styles")
	}
}

func TestStaticUIIncludesEncryptedWorkspaceArchiveSupport(t *testing.T) {
	index, err := staticFiles.ReadFile("static/index.html")
	if err != nil {
		t.Fatalf("ReadFile index returned error: %v", err)
	}
	app, err := staticFiles.ReadFile("static/app.js")
	if err != nil {
		t.Fatalf("ReadFile app returned error: %v", err)
	}
	for _, want := range []string{"exportEncryptedWorkspacesButton", "importEncryptedWorkspacesButton"} {
		if !strings.Contains(string(index), want) {
			t.Fatalf("index.html does not contain %q encrypted archive control", want)
		}
	}
	for _, want := range []string{"encryptWorkspaceArchive", "decryptWorkspaceArchive", "AES-GCM"} {
		if !strings.Contains(string(app), want) {
			t.Fatalf("app.js does not contain %q encrypted archive support", want)
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

func TestServerStartsInteractiveStream(t *testing.T) {
	server, err := NewServer()
	if err != nil {
		t.Fatalf("NewServer returned error: %v", err)
	}

	command := "printf pty-ready"
	if runtime.GOOS == "windows" {
		command = "cmd /C echo pty-ready"
	}
	body := `{"sessionId":"stream-tab","command":` + strconv.Quote(command) + `,"cols":80,"rows":24}`
	request := httptest.NewRequest(http.MethodPost, "/api/pty/start", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", response.Code, response.Body.String())
	}
	var payload PTYStartResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if !payload.OK || payload.SessionID != "stream-tab" {
		t.Fatalf("payload = %#v, want stream-tab start", payload)
	}
}

func TestServerStopsInteractiveStream(t *testing.T) {
	server, err := NewServer()
	if err != nil {
		t.Fatalf("NewServer returned error: %v", err)
	}

	command := "cmd /C more"
	if runtime.GOOS != "windows" {
		command = "cat"
	}
	body := `{"sessionId":"stop-stream","command":` + strconv.Quote(command) + `,"cols":80,"rows":24}`
	request := httptest.NewRequest(http.MethodPost, "/api/pty/start", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("start status = %d, want 200; body = %s", response.Code, response.Body.String())
	}

	stop := httptest.NewRequest(http.MethodPost, "/api/pty/stop", bytes.NewBufferString(`{"sessionId":"stop-stream"}`))
	stop.Header.Set("Content-Type", "application/json")
	stopped := httptest.NewRecorder()
	server.Handler().ServeHTTP(stopped, stop)
	if stopped.Code != http.StatusOK {
		t.Fatalf("stop status = %d, want 200; body = %s", stopped.Code, stopped.Body.String())
	}
}

func TestServerRejectsOversizedInteractiveInput(t *testing.T) {
	server, err := NewServer()
	if err != nil {
		t.Fatalf("NewServer returned error: %v", err)
	}

	response := executeRequest(t, server, `{"sessionId":"stream-a","command":"echo warmup"}`)
	if response.Code != http.StatusOK {
		t.Fatalf("warmup status = %d, want 200", response.Code)
	}

	body := `{"sessionId":"missing","data":"` + strings.Repeat("x", maxPTYInputBytes+1) + `"}`
	request := httptest.NewRequest(http.MethodPost, "/api/pty/input", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
}

func TestServerPersistsWorkspaces(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workspaces.json")
	server, err := NewServerWithOptions(ServerOptions{WorkspacePath: path})
	if err != nil {
		t.Fatalf("NewServerWithOptions returned error: %v", err)
	}

	body := `{"workspaces":[{"id":"ws-a","sessionId":"sess-a","name":"Alpha","history":[{"command":"pwd","ok":true}],"count":1,"failed":0,"closed":false,"transcript":[{"kind":"command","text":"pwd"}]}]}`
	save := httptest.NewRequest(http.MethodPut, "/api/workspaces", bytes.NewBufferString(body))
	save.Header.Set("Content-Type", "application/json")
	saved := httptest.NewRecorder()
	server.Handler().ServeHTTP(saved, save)
	if saved.Code != http.StatusOK {
		t.Fatalf("save status = %d, want 200; body = %s", saved.Code, saved.Body.String())
	}

	reloaded, err := NewServerWithOptions(ServerOptions{WorkspacePath: path})
	if err != nil {
		t.Fatalf("reload server returned error: %v", err)
	}
	load := httptest.NewRequest(http.MethodGet, "/api/workspaces", nil)
	loaded := httptest.NewRecorder()
	reloaded.Handler().ServeHTTP(loaded, load)
	if loaded.Code != http.StatusOK {
		t.Fatalf("load status = %d, want 200; body = %s", loaded.Code, loaded.Body.String())
	}
	if !strings.Contains(loaded.Body.String(), `"name":"Alpha"`) {
		t.Fatalf("body = %s, want persisted workspace", loaded.Body.String())
	}
}

func TestServerRejectsUnauthorizedAPIRequests(t *testing.T) {
	server, err := NewServerWithOptions(ServerOptions{AuthToken: "secret-token"})
	if err != nil {
		t.Fatalf("NewServerWithOptions returned error: %v", err)
	}

	unauthorized := executeRequest(t, server, `{"sessionId":"test-a","command":"echo blocked"}`)
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d, want 401", unauthorized.Code)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/execute", bytes.NewBufferString(`{"sessionId":"test-a","command":"echo allowed"}`))
	request.Header.Set("Authorization", "Bearer secret-token")
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("authorized status = %d, want 200; body = %s", response.Code, response.Body.String())
	}
}

func TestServerAcceptsAuthCookieAfterTokenQuery(t *testing.T) {
	server, err := NewServerWithOptions(ServerOptions{AuthToken: "secret-token"})
	if err != nil {
		t.Fatalf("NewServerWithOptions returned error: %v", err)
	}

	first := httptest.NewRequest(http.MethodGet, "/?token=secret-token", nil)
	firstResponse := httptest.NewRecorder()
	server.Handler().ServeHTTP(firstResponse, first)
	if firstResponse.Code != http.StatusOK {
		t.Fatalf("first status = %d, want 200; body = %s", firstResponse.Code, firstResponse.Body.String())
	}
	cookies := firstResponse.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatalf("expected auth cookie")
	}

	next := httptest.NewRequest(http.MethodGet, "/", nil)
	next.AddCookie(cookies[0])
	nextResponse := httptest.NewRecorder()
	server.Handler().ServeHTTP(nextResponse, next)
	if nextResponse.Code != http.StatusOK {
		t.Fatalf("cookie status = %d, want 200; body = %s", nextResponse.Code, nextResponse.Body.String())
	}
}

func TestServerRecoversWorkspaceShellStateAfterRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workspaces.json")
	server, err := NewServerWithOptions(ServerOptions{WorkspacePath: path})
	if err != nil {
		t.Fatalf("NewServerWithOptions returned error: %v", err)
	}

	workspace := `{"workspaces":[{"id":"ws-recover","sessionId":"recover-tab","name":"Recover","history":[],"count":0,"failed":0,"closed":false,"transcript":[]}]}`
	save := httptest.NewRequest(http.MethodPut, "/api/workspaces", bytes.NewBufferString(workspace))
	save.Header.Set("Content-Type", "application/json")
	server.Handler().ServeHTTP(httptest.NewRecorder(), save)

	for _, command := range []string{"set TARGET=recovered", "fn greet = echo hi $TARGET"} {
		response := executeRequest(t, server, `{"sessionId":"recover-tab","command":`+strconv.Quote(command)+`}`)
		if response.Code != http.StatusOK {
			t.Fatalf("command %q status = %d, want 200; body = %s", command, response.Code, response.Body.String())
		}
	}
	resync := httptest.NewRequest(http.MethodPut, "/api/workspaces", bytes.NewBufferString(workspace))
	resync.Header.Set("Content-Type", "application/json")
	resynced := httptest.NewRecorder()
	server.Handler().ServeHTTP(resynced, resync)
	if resynced.Code != http.StatusOK {
		t.Fatalf("resync status = %d, want 200; body = %s", resynced.Code, resynced.Body.String())
	}

	restarted, err := NewServerWithOptions(ServerOptions{WorkspacePath: path})
	if err != nil {
		t.Fatalf("restart server returned error: %v", err)
	}
	response := executeRequest(t, restarted, `{"sessionId":"recover-tab","command":"greet"}`)
	if response.Code != http.StatusOK {
		t.Fatalf("recovered command status = %d, want 200; body = %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "hi recovered") {
		t.Fatalf("body = %s, want recovered shell state", response.Body.String())
	}
}

func TestServerAcceptsInteractiveResize(t *testing.T) {
	server, err := NewServer()
	if err != nil {
		t.Fatalf("NewServer returned error: %v", err)
	}

	command := "cmd /C more"
	if runtime.GOOS != "windows" {
		command = "cat"
	}
	body := `{"sessionId":"resize-stream","command":` + strconv.Quote(command) + `,"cols":80,"rows":24}`
	request := httptest.NewRequest(http.MethodPost, "/api/pty/start", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("start status = %d, want 200; body = %s", response.Code, response.Body.String())
	}

	resize := httptest.NewRequest(http.MethodPost, "/api/pty/resize", bytes.NewBufferString(`{"sessionId":"resize-stream","cols":100,"rows":32}`))
	resize.Header.Set("Content-Type", "application/json")
	resized := httptest.NewRecorder()
	server.Handler().ServeHTTP(resized, resize)
	if resized.Code != http.StatusOK {
		t.Fatalf("resize status = %d, want 200; body = %s", resized.Code, resized.Body.String())
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
