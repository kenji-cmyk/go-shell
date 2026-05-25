package ui

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

const maxPTYInputBytes = 8 * 1024

type PTYManager struct {
	mu      sync.Mutex
	streams map[string]*PTYStream
}

type PTYStream struct {
	id       string
	command  string
	pty      bool
	input    io.WriteCloser
	resize   func(int, int) error
	cancel   context.CancelFunc
	messages chan string
	done     chan struct{}
}

type PTYStartRequest struct {
	SessionID string `json:"sessionId"`
	Command   string `json:"command"`
	Cols      int    `json:"cols"`
	Rows      int    `json:"rows"`
}

type PTYStartResponse struct {
	OK        bool   `json:"ok"`
	SessionID string `json:"sessionId,omitempty"`
	PTY       bool   `json:"pty"`
	Error     string `json:"error,omitempty"`
}

type PTYInputRequest struct {
	SessionID string `json:"sessionId"`
	Data      string `json:"data"`
}

type PTYResizeRequest struct {
	SessionID string `json:"sessionId"`
	Cols      int    `json:"cols"`
	Rows      int    `json:"rows"`
}

func NewPTYManager() *PTYManager {
	return &PTYManager{streams: make(map[string]*PTYStream)}
}

func (m *PTYManager) Start(id string, command string, cols int, rows int) (*PTYStream, error) {
	id = normalizeSessionID(id)
	if err := validateSessionID(id); err != nil {
		return nil, err
	}
	if command == "" {
		command = defaultInteractiveCommand()
	}

	m.mu.Lock()
	if previous, ok := m.streams[id]; ok {
		previous.Close()
	}
	m.mu.Unlock()

	stream, err := startPlatformStream(id, command, cols, rows)
	if err != nil {
		return nil, err
	}

	m.mu.Lock()
	m.streams[id] = stream
	m.mu.Unlock()

	go func() {
		<-stream.done
		m.mu.Lock()
		if m.streams[id] == stream {
			delete(m.streams, id)
		}
		m.mu.Unlock()
	}()

	return stream, nil
}

func (m *PTYManager) Get(id string) (*PTYStream, error) {
	id = normalizeSessionID(id)
	if err := validateSessionID(id); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	stream, ok := m.streams[id]
	if !ok {
		return nil, errors.New("interactive stream is not running")
	}
	return stream, nil
}

func (m *PTYManager) Write(id string, data string) error {
	if len(data) > maxPTYInputBytes {
		return errors.New("interactive input is too large")
	}
	stream, err := m.Get(id)
	if err != nil {
		return err
	}
	_, err = io.WriteString(stream.input, data)
	return err
}

func (m *PTYManager) Resize(id string, cols int, rows int) error {
	if cols <= 0 || rows <= 0 {
		return errors.New("terminal size must be positive")
	}
	stream, err := m.Get(id)
	if err != nil {
		return err
	}
	if stream.resize == nil {
		return nil
	}
	return stream.resize(cols, rows)
}

func (m *PTYManager) Stop(id string) error {
	stream, err := m.Get(id)
	if err != nil {
		return err
	}
	stream.Close()
	return nil
}

func (s *PTYStream) Close() {
	s.cancel()
	_ = s.input.Close()
}

func startPipeStream(id string, command string) (*PTYStream, error) {
	ctx, cancel := context.WithCancel(context.Background())
	cmd := interactiveProcess(ctx, command)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("open interactive stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("open interactive stdout: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("open interactive stderr: %w", err)
	}

	stream := &PTYStream{
		id:       id,
		command:  command,
		pty:      false,
		input:    stdin,
		cancel:   cancel,
		messages: make(chan string, 128),
		done:     make(chan struct{}),
	}

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("start interactive stream: %w", err)
	}

	var readers sync.WaitGroup
	readers.Add(2)
	go copyStream(&readers, stream.messages, stdout)
	go copyStream(&readers, stream.messages, stderr)
	go func() {
		err := cmd.Wait()
		readers.Wait()
		if err != nil && ctx.Err() == nil {
			stream.messages <- fmt.Sprintf("\n[process exited: %v]\n", err)
		} else {
			stream.messages <- "\n[process exited]\n"
		}
		close(stream.messages)
		close(stream.done)
		cancel()
	}()

	return stream, nil
}

func copyStream(wg *sync.WaitGroup, messages chan<- string, reader io.Reader) {
	defer wg.Done()
	buf := make([]byte, 4096)
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			messages <- string(buf[:n])
		}
		if err != nil {
			return
		}
	}
}

func interactiveProcess(ctx context.Context, command string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.CommandContext(ctx, "cmd", "/C", command)
	}
	return exec.CommandContext(ctx, "sh", "-lc", command)
}

func defaultInteractiveCommand() string {
	if runtime.GOOS == "windows" {
		if value := os.Getenv("COMSPEC"); value != "" {
			return value
		}
		return "cmd"
	}
	if value := os.Getenv("SHELL"); value != "" {
		return value
	}
	return "sh"
}

func streamServerSentEvents(w io.Writer, flusher httpFlusher, messages <-chan string, done <-chan struct{}) {
	keepAlive := time.NewTicker(15 * time.Second)
	defer keepAlive.Stop()
	writer := bufio.NewWriter(w)

	for {
		select {
		case message, ok := <-messages:
			if !ok {
				_, _ = writer.WriteString("event: close\ndata: done\n\n")
				_ = writer.Flush()
				flusher.Flush()
				return
			}
			writeSSEData(writer, "output", message)
			_ = writer.Flush()
			flusher.Flush()
		case <-keepAlive.C:
			_, _ = writer.WriteString(": keepalive\n\n")
			_ = writer.Flush()
			flusher.Flush()
		case <-done:
			return
		}
	}
}

func writeSSEData(writer *bufio.Writer, event string, data string) {
	_, _ = fmt.Fprintf(writer, "event: %s\n", event)
	scanner := bufio.NewScanner(strings.NewReader(data))
	scanner.Buffer(make([]byte, 0, 1024), maxPTYInputBytes)
	wrote := false
	for scanner.Scan() {
		wrote = true
		_, _ = fmt.Fprintf(writer, "data: %s\n", scanner.Text())
	}
	if !wrote || strings.HasSuffix(data, "\n") {
		_, _ = writer.WriteString("data: \n")
	}
	_, _ = writer.WriteString("\n")
}

type httpFlusher interface {
	Flush()
}
