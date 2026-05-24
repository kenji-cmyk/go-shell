package executor

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

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

func TestRunExpandsWildcards(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first.txt")
	second := filepath.Join(dir, "second.txt")
	if err := os.WriteFile(first, []byte("one"), 0644); err != nil {
		t.Fatalf("WriteFile first returned error: %v", err)
	}
	if err := os.WriteFile(second, []byte("two"), 0644); err != nil {
		t.Fatalf("WriteFile second returned error: %v", err)
	}

	var out bytes.Buffer
	exec := Executor{Out: &out, Err: &bytes.Buffer{}}
	err := exec.Run(parser.Command{
		Name: "cmd",
		Args: []string{"/C", "echo", filepath.Join(dir, "*.txt")},
		Raw:  "cmd /C echo " + filepath.Join(dir, "*.txt"),
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, first) || !strings.Contains(output, second) {
		t.Fatalf("output = %q, want both wildcard matches", output)
	}
}

func TestRunLineBackgroundReturnsImmediately(t *testing.T) {
	var out bytes.Buffer
	exec := Executor{Out: &out, Err: &bytes.Buffer{}}

	line, err := parser.ParseLine(`cmd /C rem background > NUL &`)
	if err != nil {
		t.Fatalf("ParseLine returned error: %v", err)
	}

	if err := exec.RunLine(line); err != nil {
		t.Fatalf("RunLine returned error: %v", err)
	}
	if !strings.Contains(out.String(), "[background]") {
		t.Fatalf("output = %q, want background job notice", out.String())
	}
}

func TestRunLineForwardsInterruptToForegroundProcess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows console control events are not reliable under go test")
	}

	signals := make(chan os.Signal, 1)
	exec := Executor{Out: &bytes.Buffer{}, Err: &bytes.Buffer{}, Signals: signals}
	line, err := parser.ParseLine(`cmd /C ping 127.0.0.1 -n 6 > NUL`)
	if err != nil {
		t.Fatalf("ParseLine returned error: %v", err)
	}

	errs := make(chan error, 1)
	start := time.Now()
	go func() {
		errs <- exec.RunLine(line)
	}()

	time.Sleep(200 * time.Millisecond)
	signals <- os.Interrupt

	select {
	case <-errs:
		if time.Since(start) > 4*time.Second {
			t.Fatal("foreground process did not stop promptly after interrupt")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("foreground process did not receive forwarded interrupt")
	}
}

func TestJobTableCanMarkStoppedAndResume(t *testing.T) {
	jobs := NewJobTable()
	job := jobs.Add("demo", nil)

	if err := jobs.MarkStopped(job.ID); err != nil {
		t.Fatalf("MarkStopped returned error: %v", err)
	}
	stopped, ok := jobs.Get(job.ID)
	if !ok {
		t.Fatal("job not found")
	}
	if stopped.Status != JobStopped {
		t.Fatalf("status = %q, want stopped", stopped.Status)
	}

	if err := jobs.MarkRunning(job.ID); err != nil {
		t.Fatalf("MarkRunning returned error: %v", err)
	}
	running, ok := jobs.Get(job.ID)
	if !ok {
		t.Fatal("job not found after resume")
	}
	if running.Status != JobRunning {
		t.Fatalf("status = %q, want running", running.Status)
	}
}

func TestJobTableResumeDoneJobReturnsError(t *testing.T) {
	jobs := NewJobTable()
	job := jobs.Add("demo", nil)
	jobs.Complete(job.ID, nil)

	err := jobs.MarkRunning(job.ID)
	if err == nil {
		t.Fatal("MarkRunning returned nil error for done job")
	}
	if !errors.Is(err, ErrJobNotStopped) {
		t.Fatalf("error = %v, want ErrJobNotStopped", err)
	}
}
