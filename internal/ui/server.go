package ui

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"go-shell/internal/shell"
)

//go:embed static/*
var staticFiles embed.FS

const maxCommandBytes = 16 * 1024

var shellProcessMu sync.Mutex

type Server struct {
	mux        *http.ServeMux
	mu         sync.Mutex
	sessions   map[string]*Session
	pty        *PTYManager
	authToken  string
	workspaces *WorkspaceStore
}

type Session struct {
	mu     sync.Mutex
	out    *safeBuffer
	err    *safeBuffer
	shell  *shell.Shell
	state  shell.State
	closed bool
}

type ServerOptions struct {
	AuthToken              string
	WorkspacePath          string
	MaxWorkspaceHistory    int
	MaxWorkspaceTranscript int
}

type ExecuteRequest struct {
	SessionID string `json:"sessionId"`
	Command   string `json:"command"`
}

type ExecuteResponse struct {
	OK          bool   `json:"ok"`
	KeepRunning bool   `json:"keepRunning"`
	Stdout      string `json:"stdout"`
	Stderr      string `json:"stderr"`
	Error       string `json:"error,omitempty"`
}

type safeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func NewServer() (*Server, error) {
	return NewServerWithOptions(ServerOptions{
		AuthToken:     os.Getenv("GOSH_UI_TOKEN"),
		WorkspacePath: os.Getenv("GOSH_WORKSPACES_FILE"),
	})
}

func NewServerWithOptions(options ServerOptions) (*Server, error) {
	static, err := fs.Sub(staticFiles, "static")
	if err != nil {
		return nil, fmt.Errorf("load static ui: %w", err)
	}
	storePath := strings.TrimSpace(options.WorkspacePath)
	if storePath == "" {
		storePath = defaultWorkspacePath()
	}
	store, err := NewWorkspaceStoreWithLimits(storePath, WorkspaceLimits{
		MaxHistory:    options.MaxWorkspaceHistory,
		MaxTranscript: options.MaxWorkspaceTranscript,
	})
	if err != nil {
		return nil, err
	}

	server := &Server{
		mux:        http.NewServeMux(),
		sessions:   make(map[string]*Session),
		pty:        NewPTYManager(),
		authToken:  strings.TrimSpace(options.AuthToken),
		workspaces: store,
	}
	server.mux.Handle("GET /", server.withAuth(http.FileServer(http.FS(static))))
	server.mux.Handle("GET /api/workspaces", server.withAuth(http.HandlerFunc(server.handleWorkspaces)))
	server.mux.Handle("PUT /api/workspaces", server.withAuth(http.HandlerFunc(server.handleWorkspaces)))
	server.mux.Handle("POST /api/execute", server.withAuth(http.HandlerFunc(server.handleExecute)))
	server.mux.Handle("POST /api/pty/start", server.withAuth(http.HandlerFunc(server.handlePTYStart)))
	server.mux.Handle("POST /api/pty/input", server.withAuth(http.HandlerFunc(server.handlePTYInput)))
	server.mux.Handle("POST /api/pty/resize", server.withAuth(http.HandlerFunc(server.handlePTYResize)))
	server.mux.Handle("POST /api/pty/stop", server.withAuth(http.HandlerFunc(server.handlePTYStop)))
	server.mux.Handle("GET /api/pty/stream", server.withAuth(http.HandlerFunc(server.handlePTYStream)))
	return server, nil
}

func NewSession() *Session {
	return newSessionWithState(nil)
}

func newSessionWithState(state *shell.State) *Session {
	out := &safeBuffer{}
	errOut := &safeBuffer{}
	session := &Session{
		out:   out,
		err:   errOut,
		shell: shell.New(strings.NewReader(""), out, errOut),
	}
	if state != nil {
		_ = session.shell.Restore(*state)
	}
	session.state = session.shell.Snapshot()
	return session
}

func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) withAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := requestToken(r)
		if s.authToken == "" || token == s.authToken {
			if token == s.authToken && strings.TrimSpace(r.URL.Query().Get("token")) != "" {
				http.SetCookie(w, &http.Cookie{
					Name:     "gosh_ui_token",
					Value:    token,
					Path:     "/",
					HttpOnly: true,
					SameSite: http.SameSiteLaxMode,
				})
			}
			next.ServeHTTP(w, r)
			return
		}
		writeJSON(w, http.StatusUnauthorized, ExecuteResponse{OK: false, Error: "unauthorized"})
	})
}

func requestToken(r *http.Request) string {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(strings.ToLower(header), "bearer ") {
		return strings.TrimSpace(header[7:])
	}
	if token := strings.TrimSpace(r.Header.Get("X-Gosh-Token")); token != "" {
		return token
	}
	if token := strings.TrimSpace(r.URL.Query().Get("token")); token != "" {
		return token
	}
	cookie, err := r.Cookie("gosh_ui_token")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(cookie.Value)
}

func (s *Server) handleWorkspaces(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		payload, err := s.workspaces.Load()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, ExecuteResponse{OK: false, Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, payload)
	case http.MethodPut:
		reader := http.MaxBytesReader(w, r.Body, maxWorkspaceBytes)
		defer reader.Close()
		var payload WorkspacePayload
		if err := json.NewDecoder(reader).Decode(&payload); err != nil {
			writeJSON(w, http.StatusBadRequest, ExecuteResponse{OK: false, Error: "invalid JSON request"})
			return
		}
		if err := s.workspaces.Save(payload); err != nil {
			writeJSON(w, http.StatusBadRequest, ExecuteResponse{OK: false, Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, ExecuteResponse{OK: true})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, ExecuteResponse{OK: false, Error: "method not allowed"})
	}
}

func (s *Server) handleExecute(w http.ResponseWriter, r *http.Request) {
	var request ExecuteRequest
	reader := http.MaxBytesReader(w, r.Body, maxCommandBytes)
	defer reader.Close()

	if err := json.NewDecoder(reader).Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, ExecuteResponse{OK: false, Error: "invalid JSON request"})
		return
	}

	session, err := s.session(request.SessionID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ExecuteResponse{OK: false, Error: err.Error()})
		return
	}

	response := session.Execute(request.Command)
	if response.KeepRunning {
		if err := s.workspaces.SaveShellState(normalizeSessionID(request.SessionID), session.Snapshot()); err != nil {
			writeJSON(w, http.StatusInternalServerError, ExecuteResponse{OK: false, KeepRunning: response.KeepRunning, Error: err.Error()})
			return
		}
	}
	status := http.StatusOK
	if !response.OK {
		status = http.StatusBadRequest
	}
	writeJSON(w, status, response)
}

func defaultWorkspacePath() string {
	dir, err := os.UserConfigDir()
	if err != nil || strings.TrimSpace(dir) == "" {
		return ""
	}
	return filepath.Join(dir, "gosh", "workspaces.json")
}

func (s *Server) session(id string) (*Session, error) {
	id = normalizeSessionID(id)
	if err := validateSessionID(id); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[id]
	if !ok {
		state, err := s.workspaces.LoadShellState(id)
		if err != nil {
			return nil, err
		}
		shellProcessMu.Lock()
		session = newSessionWithState(state)
		shellProcessMu.Unlock()
		s.sessions[id] = session
	}
	return session, nil
}

func (s *Server) handlePTYStart(w http.ResponseWriter, r *http.Request) {
	var request PTYStartRequest
	reader := http.MaxBytesReader(w, r.Body, maxCommandBytes)
	defer reader.Close()

	if err := json.NewDecoder(reader).Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, PTYStartResponse{OK: false, Error: "invalid JSON request"})
		return
	}

	stream, err := s.pty.Start(request.SessionID, strings.TrimSpace(request.Command), request.Cols, request.Rows)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, PTYStartResponse{OK: false, Error: err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, PTYStartResponse{OK: true, SessionID: stream.id, PTY: stream.pty})
}

func (s *Server) handlePTYInput(w http.ResponseWriter, r *http.Request) {
	var request PTYInputRequest
	reader := http.MaxBytesReader(w, r.Body, maxPTYInputBytes)
	defer reader.Close()

	if err := json.NewDecoder(reader).Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, ExecuteResponse{OK: false, Error: "invalid JSON request"})
		return
	}
	if err := s.pty.Write(request.SessionID, request.Data); err != nil {
		writeJSON(w, http.StatusBadRequest, ExecuteResponse{OK: false, Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, ExecuteResponse{OK: true, KeepRunning: true})
}

func (s *Server) handlePTYResize(w http.ResponseWriter, r *http.Request) {
	var request PTYResizeRequest
	reader := http.MaxBytesReader(w, r.Body, maxCommandBytes)
	defer reader.Close()

	if err := json.NewDecoder(reader).Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, ExecuteResponse{OK: false, Error: "invalid JSON request"})
		return
	}
	if err := s.pty.Resize(request.SessionID, request.Cols, request.Rows); err != nil {
		writeJSON(w, http.StatusBadRequest, ExecuteResponse{OK: false, Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, ExecuteResponse{OK: true, KeepRunning: true})
}

func (s *Server) handlePTYStop(w http.ResponseWriter, r *http.Request) {
	var request PTYInputRequest
	reader := http.MaxBytesReader(w, r.Body, maxCommandBytes)
	defer reader.Close()

	if err := json.NewDecoder(reader).Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, ExecuteResponse{OK: false, Error: "invalid JSON request"})
		return
	}
	if err := s.pty.Stop(request.SessionID); err != nil {
		writeJSON(w, http.StatusBadRequest, ExecuteResponse{OK: false, Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, ExecuteResponse{OK: true, KeepRunning: false})
}

func (s *Server) handlePTYStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, ExecuteResponse{OK: false, Error: "streaming is not supported"})
		return
	}
	stream, err := s.pty.Get(r.URL.Query().Get("sessionId"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ExecuteResponse{OK: false, Error: err.Error()})
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	streamServerSentEvents(w, flusher, stream.messages, r.Context().Done())
}

func (s *Session) Execute(command string) ExecuteResponse {
	s.mu.Lock()
	defer s.mu.Unlock()

	command = strings.TrimRight(command, "\r\n")
	if strings.TrimSpace(command) == "" {
		return ExecuteResponse{OK: false, KeepRunning: !s.closed, Error: "command is required"}
	}
	if len(command) > maxCommandBytes {
		return ExecuteResponse{OK: false, KeepRunning: !s.closed, Error: "command is too large"}
	}
	if s.closed {
		return ExecuteResponse{OK: false, KeepRunning: false, Error: "session has exited"}
	}

	s.out.Reset()
	s.err.Reset()
	shellProcessMu.Lock()
	defer shellProcessMu.Unlock()
	if err := s.shell.Restore(s.state); err != nil {
		return ExecuteResponse{OK: false, KeepRunning: !s.closed, Error: err.Error()}
	}
	keepRunning := s.shell.ExecuteLine(command)
	s.state = s.shell.Snapshot()
	if !keepRunning {
		s.closed = true
	}

	stderr := s.err.String()
	return ExecuteResponse{
		OK:          stderr == "",
		KeepRunning: keepRunning,
		Stdout:      s.out.String(),
		Stderr:      stderr,
	}
}

func (s *Session) Snapshot() shell.State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

func normalizeSessionID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return "default"
	}
	return id
}

func validateSessionID(id string) error {
	if len(id) > 128 {
		return fmt.Errorf("session id is too large")
	}
	for _, r := range id {
		if r == '-' || r == '_' || r >= '0' && r <= '9' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' {
			continue
		}
		return fmt.Errorf("session id contains invalid characters")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func (b *safeBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *safeBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func (b *safeBuffer) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf.Reset()
}

var _ io.Writer = (*safeBuffer)(nil)
