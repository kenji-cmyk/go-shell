package ui

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	maxWorkspaceBytes       = 2 * 1024 * 1024
	maxPersistedWorkspaces  = 64
	maxWorkspaceHistory     = 1000
	maxWorkspaceTranscript  = 400
	maxWorkspaceStringBytes = 64 * 1024
)

type WorkspaceStore struct {
	mu   sync.Mutex
	path string
}

type WorkspacePayload struct {
	Workspaces []WorkspaceRecord `json:"workspaces"`
}

type WorkspaceRecord struct {
	ID         string             `json:"id"`
	SessionID  string             `json:"sessionId"`
	Name       string             `json:"name"`
	History    []HistoryRecord    `json:"history"`
	Count      int                `json:"count"`
	Failed     int                `json:"failed"`
	Closed     bool               `json:"closed"`
	Transcript []TranscriptRecord `json:"transcript"`
}

type HistoryRecord struct {
	Command string `json:"command"`
	OK      bool   `json:"ok"`
	Stdout  string `json:"stdout"`
	Stderr  string `json:"stderr"`
	At      string `json:"at"`
}

type TranscriptRecord struct {
	Kind string `json:"kind"`
	Text string `json:"text"`
}

func NewWorkspaceStore(path string) (*WorkspaceStore, error) {
	path = strings.TrimSpace(path)
	if path != "" {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return nil, fmt.Errorf("create workspace store directory: %w", err)
		}
	}
	return &WorkspaceStore{path: path}, nil
}

func (s *WorkspaceStore) Load() (WorkspacePayload, error) {
	if s.path == "" {
		return WorkspacePayload{}, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return WorkspacePayload{}, nil
	}
	if err != nil {
		return WorkspacePayload{}, fmt.Errorf("read workspaces: %w", err)
	}
	var payload WorkspacePayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return WorkspacePayload{}, fmt.Errorf("parse workspaces: %w", err)
	}
	return sanitizeWorkspacePayload(payload)
}

func (s *WorkspaceStore) Save(payload WorkspacePayload) error {
	clean, err := sanitizeWorkspacePayload(payload)
	if err != nil {
		return err
	}
	if s.path == "" {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.MarshalIndent(clean, "", "  ")
	if err != nil {
		return fmt.Errorf("encode workspaces: %w", err)
	}
	temp := s.path + ".tmp"
	if err := os.WriteFile(temp, data, 0o600); err != nil {
		return fmt.Errorf("write workspaces: %w", err)
	}
	if err := os.Rename(temp, s.path); err != nil {
		_ = os.Remove(temp)
		return fmt.Errorf("replace workspaces: %w", err)
	}
	return nil
}

func sanitizeWorkspacePayload(payload WorkspacePayload) (WorkspacePayload, error) {
	if len(payload.Workspaces) > maxPersistedWorkspaces {
		payload.Workspaces = payload.Workspaces[:maxPersistedWorkspaces]
	}
	clean := WorkspacePayload{Workspaces: make([]WorkspaceRecord, 0, len(payload.Workspaces))}
	for _, workspace := range payload.Workspaces {
		if err := validateSessionID(normalizeSessionID(workspace.SessionID)); err != nil {
			return WorkspacePayload{}, fmt.Errorf("workspace session id is invalid: %w", err)
		}
		id := strings.TrimSpace(workspace.ID)
		if id == "" {
			id = normalizeSessionID(workspace.SessionID)
		}
		if err := validateSessionID(id); err != nil {
			return WorkspacePayload{}, fmt.Errorf("workspace id is invalid: %w", err)
		}
		name := strings.TrimSpace(limitWorkspaceString(workspace.Name))
		if name == "" {
			name = "Workspace"
		}
		workspace.ID = id
		workspace.SessionID = normalizeSessionID(workspace.SessionID)
		workspace.Name = name
		workspace.History = sanitizeHistory(workspace.History)
		workspace.Transcript = sanitizeTranscript(workspace.Transcript)
		if workspace.Count < 0 {
			workspace.Count = 0
		}
		if workspace.Failed < 0 {
			workspace.Failed = 0
		}
		clean.Workspaces = append(clean.Workspaces, workspace)
	}
	return clean, nil
}

func sanitizeHistory(records []HistoryRecord) []HistoryRecord {
	if len(records) > maxWorkspaceHistory {
		records = records[len(records)-maxWorkspaceHistory:]
	}
	clean := make([]HistoryRecord, 0, len(records))
	for _, record := range records {
		record.Command = strings.TrimSpace(limitWorkspaceString(record.Command))
		record.Stdout = limitWorkspaceString(record.Stdout)
		record.Stderr = limitWorkspaceString(record.Stderr)
		record.At = strings.TrimSpace(limitWorkspaceString(record.At))
		if record.Command != "" {
			clean = append(clean, record)
		}
	}
	return clean
}

func sanitizeTranscript(records []TranscriptRecord) []TranscriptRecord {
	if len(records) > maxWorkspaceTranscript {
		records = records[len(records)-maxWorkspaceTranscript:]
	}
	clean := make([]TranscriptRecord, 0, len(records))
	for _, record := range records {
		record.Kind = strings.TrimSpace(limitWorkspaceString(record.Kind))
		record.Text = limitWorkspaceString(record.Text)
		if isTranscriptKind(record.Kind) && record.Text != "" {
			clean = append(clean, record)
		}
	}
	return clean
}

func isTranscriptKind(kind string) bool {
	return kind == "command" || kind == "stdout" || kind == "stderr"
}

func limitWorkspaceString(value string) string {
	if len(value) > maxWorkspaceStringBytes {
		return value[:maxWorkspaceStringBytes]
	}
	return value
}
