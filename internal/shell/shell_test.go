package shell

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExecuteLineHandlesEmptyInput(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	sh := New(strings.NewReader(""), &out, &errOut)

	if !sh.ExecuteLine("   ") {
		t.Fatal("empty input stopped the shell")
	}
	if errOut.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", errOut.String())
	}
}

func TestExecuteLineStopsOnExit(t *testing.T) {
	sh := New(strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})

	if sh.ExecuteLine("exit") {
		t.Fatal("exit did not stop the shell")
	}
}

func TestRunProcessesInputUntilExit(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	sh := New(strings.NewReader("echo hello\nexit\n"), &out, &errOut)

	if err := sh.Run(); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !strings.Contains(out.String(), "hello") {
		t.Fatalf("stdout = %q, want hello", out.String())
	}
	if errOut.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", errOut.String())
	}
}

func TestExecuteLineRedirectsBuiltinOutput(t *testing.T) {
	dir := t.TempDir()
	output := filepath.Join(dir, "echo.txt")
	var out bytes.Buffer
	var errOut bytes.Buffer
	sh := New(strings.NewReader(""), &out, &errOut)

	if !sh.ExecuteLine(`echo hello > ` + output) {
		t.Fatal("echo stopped the shell")
	}
	if out.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", out.String())
	}
	if errOut.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", errOut.String())
	}

	content, err := os.ReadFile(output)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if strings.TrimSpace(string(content)) != "hello" {
		t.Fatalf("file content = %q, want hello", string(content))
	}
}

func TestPromptUsesEnvironmentTemplate(t *testing.T) {
	t.Setenv("GOSH_PROMPT", "[{base}]$ ")

	sh := New(strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	prompt := sh.prompt()
	if !strings.HasPrefix(prompt, "[") || !strings.HasSuffix(prompt, "]$ ") {
		t.Fatalf("prompt = %q, want configured template", prompt)
	}
}
