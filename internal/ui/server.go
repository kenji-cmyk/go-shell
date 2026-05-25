package ui

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"strings"
	"sync"

	"go-shell/internal/shell"
)

//go:embed static/*
var staticFiles embed.FS

const maxCommandBytes = 16 * 1024

type Server struct {
	mux      *http.ServeMux
	mu       sync.Mutex
	sessions map[string]*Session
	pty      *PTYManager
}

type Session struct {
	mu     sync.Mutex
	out    *safeBuffer
	err    *safeBuffer
	shell  *shell.Shell
	closed bool
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
	static, err := fs.Sub(staticFiles, "static")
	if err != nil {
		return nil, fmt.Errorf("load static ui: %w", err)
	}

	server := &Server{
		mux:      http.NewServeMux(),
		sessions: make(map[string]*Session),
		pty:      NewPTYManager(),
	}
	server.mux.Handle("GET /", http.FileServer(http.FS(static)))
	server.mux.HandleFunc("POST /api/execute", server.handleExecute)
	server.mux.HandleFunc("POST /api/pty/start", server.handlePTYStart)
	server.mux.HandleFunc("POST /api/pty/input", server.handlePTYInput)
	server.mux.HandleFunc("POST /api/pty/resize", server.handlePTYResize)
	server.mux.HandleFunc("POST /api/pty/stop", server.handlePTYStop)
	server.mux.HandleFunc("GET /api/pty/stream", server.handlePTYStream)
	return server, nil
}

func NewSession() *Session {
	out := &safeBuffer{}
	errOut := &safeBuffer{}
	return &Session{
		out:   out,
		err:   errOut,
		shell: shell.New(strings.NewReader(""), out, errOut),
	}
}

func (s *Server) Handler() http.Handler {
	return s.mux
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
	status := http.StatusOK
	if !response.OK {
		status = http.StatusBadRequest
	}
	writeJSON(w, status, response)
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
		session = NewSession()
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
	keepRunning := s.shell.ExecuteLine(command)
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
