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
	mux     *http.ServeMux
	session *Session
}

type Session struct {
	mu     sync.Mutex
	out    *safeBuffer
	err    *safeBuffer
	shell  *shell.Shell
	closed bool
}

type ExecuteRequest struct {
	Command string `json:"command"`
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
		mux:     http.NewServeMux(),
		session: NewSession(),
	}
	server.mux.Handle("GET /", http.FileServer(http.FS(static)))
	server.mux.HandleFunc("POST /api/execute", server.handleExecute)
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

	response := s.session.Execute(request.Command)
	status := http.StatusOK
	if !response.OK {
		status = http.StatusBadRequest
	}
	writeJSON(w, status, response)
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

func writeJSON(w http.ResponseWriter, status int, value ExecuteResponse) {
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
