package executor

import (
	"bytes"
	"os"
	"path/filepath"
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

func TestRunLinePipeline(t *testing.T) {
	var out bytes.Buffer
	exec := Executor{Out: &out, Err: &bytes.Buffer{}}

	line, err := parser.ParseLine(`cmd /C echo hello | findstr hello`)
	if err != nil {
		t.Fatalf("ParseLine returned error: %v", err)
	}

	if err := exec.RunLine(line); err != nil {
		t.Fatalf("RunLine returned error: %v", err)
	}
	if strings.TrimSpace(out.String()) != "hello" {
		t.Fatalf("output = %q, want hello", out.String())
	}
}

func TestRunLineOutputRedirect(t *testing.T) {
	dir := t.TempDir()
	output := filepath.Join(dir, "out.txt")
	exec := Executor{Out: &bytes.Buffer{}, Err: &bytes.Buffer{}}

	line, err := parser.ParseLine(`cmd /C echo hello > ` + output)
	if err != nil {
		t.Fatalf("ParseLine returned error: %v", err)
	}

	if err := exec.RunLine(line); err != nil {
		t.Fatalf("RunLine returned error: %v", err)
	}

	content, err := os.ReadFile(output)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if strings.TrimSpace(string(content)) != "hello" {
		t.Fatalf("file content = %q, want hello", string(content))
	}
}

func TestRunLineInputRedirect(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "in.txt")
	if err := os.WriteFile(input, []byte("hello\nworld\n"), 0644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	var out bytes.Buffer
	exec := Executor{Out: &out, Err: &bytes.Buffer{}}
	line, err := parser.ParseLine(`findstr hello < ` + input)
	if err != nil {
		t.Fatalf("ParseLine returned error: %v", err)
	}

	if err := exec.RunLine(line); err != nil {
		t.Fatalf("RunLine returned error: %v", err)
	}
	if strings.TrimSpace(out.String()) != "hello" {
		t.Fatalf("output = %q, want hello", out.String())
	}
}
