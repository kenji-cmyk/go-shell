package shell

import (
	"bytes"
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
