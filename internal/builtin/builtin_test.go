package builtin

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go-shell/internal/parser"
)

func TestPwdPrintsCurrentDirectory(t *testing.T) {
	var out bytes.Buffer
	reg := NewRegistry()

	handled, err := reg.Run(Context{Out: &out}, parser.Command{Name: "pwd"})
	if err != nil {
		t.Fatalf("pwd returned error: %v", err)
	}
	if !handled {
		t.Fatal("pwd was not handled")
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd returned error: %v", err)
	}
	if strings.TrimSpace(out.String()) != wd {
		t.Fatalf("pwd output = %q, want %q", strings.TrimSpace(out.String()), wd)
	}
}

func TestCdChangesDirectory(t *testing.T) {
	original, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd returned error: %v", err)
	}
	defer os.Chdir(original)

	target := t.TempDir()
	reg := NewRegistry()
	handled, err := reg.Run(Context{Out: &bytes.Buffer{}}, parser.Command{Name: "cd", Args: []string{target}})
	if err != nil {
		t.Fatalf("cd returned error: %v", err)
	}
	if !handled {
		t.Fatal("cd was not handled")
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd returned error: %v", err)
	}
	if filepath.Clean(wd) != filepath.Clean(target) {
		t.Fatalf("working directory = %q, want %q", wd, target)
	}
}

func TestCdMissingDirectoryReturnsError(t *testing.T) {
	reg := NewRegistry()
	_, err := reg.Run(Context{Out: &bytes.Buffer{}}, parser.Command{Name: "cd", Args: []string{filepath.Join(t.TempDir(), "missing")}})
	if err == nil {
		t.Fatal("cd returned nil error for missing directory")
	}
}

func TestExitSignalsShellStop(t *testing.T) {
	reg := NewRegistry()
	handled, err := reg.Run(Context{Out: &bytes.Buffer{}}, parser.Command{Name: "exit"})
	if !handled {
		t.Fatal("exit was not handled")
	}
	if !errors.Is(err, ErrExit) {
		t.Fatalf("exit error = %v, want ErrExit", err)
	}
}
