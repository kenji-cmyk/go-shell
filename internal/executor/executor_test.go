package executor

import (
	"bytes"
	"strings"
	"testing"

	"go-shell/internal/parser"
)

func TestRunCommand(t *testing.T) {
	var out bytes.Buffer
	exec := Executor{Out: &out, Err: &bytes.Buffer{}}

	err := exec.Run(parser.Command{
		Name: "cmd",
		Args: []string{"/C", "echo", "hello"},
		Raw:  "cmd /C echo hello",
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if strings.TrimSpace(out.String()) != "hello" {
		t.Fatalf("output = %q, want hello", out.String())
	}
}

func TestRunCommandNotFound(t *testing.T) {
	exec := Executor{Out: &bytes.Buffer{}, Err: &bytes.Buffer{}}

	err := exec.Run(parser.Command{
		Name: "definitely-not-a-real-command-for-gosh",
		Raw:  "definitely-not-a-real-command-for-gosh",
	})
	if err == nil {
		t.Fatal("Run returned nil error for missing command")
	}
	if !strings.Contains(err.Error(), "command not found") {
		t.Fatalf("error = %q, want command not found", err.Error())
	}
}
