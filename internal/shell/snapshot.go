package shell

import (
	"fmt"
	"os"
	"time"
)

type State struct {
	WorkingDir        string            `json:"workingDir,omitempty"`
	Variables         map[string]string `json:"variables,omitempty"`
	Functions         map[string]string `json:"functions,omitempty"`
	LastStatus        int               `json:"lastStatus"`
	LastDurationNanos int64             `json:"lastDurationNanos"`
}

func (s *Shell) Snapshot() State {
	wd, _ := os.Getwd()
	return State{
		WorkingDir:        wd,
		Variables:         copyStringMap(s.variables),
		Functions:         copyStringMap(s.functions),
		LastStatus:        s.lastStatus,
		LastDurationNanos: int64(s.lastDuration),
	}
}

func (s *Shell) Restore(state State) error {
	if state.WorkingDir != "" {
		if err := os.Chdir(state.WorkingDir); err != nil {
			return fmt.Errorf("restore working directory: %w", err)
		}
	}
	s.variables = copyStringMap(state.Variables)
	s.functions = copyStringMap(state.Functions)
	s.lastStatus = state.LastStatus
	s.lastDuration = time.Duration(state.LastDurationNanos)
	return nil
}

func copyStringMap(values map[string]string) map[string]string {
	copied := make(map[string]string, len(values))
	for key, value := range values {
		copied[key] = value
	}
	return copied
}
