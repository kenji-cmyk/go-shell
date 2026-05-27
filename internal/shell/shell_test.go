package shell

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go-shell/internal/executor"
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

func TestExecuteLineExpandsAliasFromEnvironment(t *testing.T) {
	t.Setenv("GOSH_ALIASES", "hi=echo hello")
	t.Setenv("GOSH_ALIASES_FILE", filepath.Join(t.TempDir(), "missing-aliases"))

	var out bytes.Buffer
	var errOut bytes.Buffer
	sh := New(strings.NewReader(""), &out, &errOut)

	if !sh.ExecuteLine("hi world") {
		t.Fatal("alias stopped the shell")
	}
	if strings.TrimSpace(out.String()) != "hello world" {
		t.Fatalf("stdout = %q, want hello world", out.String())
	}
	if errOut.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", errOut.String())
	}
}

func TestExecuteLineReportsSyntaxErrorPosition(t *testing.T) {
	var errOut bytes.Buffer
	sh := New(strings.NewReader(""), &bytes.Buffer{}, &errOut)

	if !sh.ExecuteLine(`echo "hello`) {
		t.Fatal("syntax error stopped the shell")
	}
	if !strings.Contains(errOut.String(), "column 6") || !strings.Contains(errOut.String(), "^") {
		t.Fatalf("stderr = %q, want column and caret", errOut.String())
	}
}

func TestRunStartupScriptFromEnvironment(t *testing.T) {
	t.Setenv("GOSH_STARTUP", "echo startup")
	t.Setenv("GOSH_STARTUP_FILE", filepath.Join(t.TempDir(), "missing-startup"))

	var out bytes.Buffer
	var errOut bytes.Buffer
	sh := New(strings.NewReader("exit\n"), &out, &errOut)

	if err := sh.Run(); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !strings.Contains(out.String(), "startup") {
		t.Fatalf("stdout = %q, want startup output", out.String())
	}
	if errOut.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", errOut.String())
	}
}

func TestPromptShowsStatusAndDuration(t *testing.T) {
	t.Setenv("GOSH_PROMPT", "{status}:{duration}> ")

	var errOut bytes.Buffer
	sh := New(strings.NewReader(""), &bytes.Buffer{}, &errOut)

	sh.ExecuteLine(`echo "unterminated`)
	prompt := sh.prompt()
	if !strings.HasPrefix(prompt, "2:") || !strings.HasSuffix(prompt, "> ") {
		t.Fatalf("prompt = %q, want status and duration", prompt)
	}
}

func TestJobsBuiltinListsBackgroundJobs(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	sh := New(strings.NewReader(""), &out, &errOut)

	if !sh.ExecuteLine("cmd /C ping 127.0.0.1 -n 2 > NUL &") {
		t.Fatal("background command stopped shell")
	}
	if !sh.ExecuteLine("jobs") {
		t.Fatal("jobs stopped shell")
	}
	if !strings.Contains(out.String(), "[1]") || !strings.Contains(out.String(), "running") {
		t.Fatalf("stdout = %q, want running job", out.String())
	}
	if errOut.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", errOut.String())
	}
}

func TestBgBuiltinReportsRunningJob(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	sh := New(strings.NewReader(""), &out, &errOut)

	sh.ExecuteLine("cmd /C ping 127.0.0.1 -n 2 > NUL &")
	if !sh.ExecuteLine("bg %1") {
		t.Fatal("bg stopped shell")
	}
	if !strings.Contains(out.String(), "already running") {
		t.Fatalf("stdout = %q, want already running", out.String())
	}
}

func TestFgBuiltinWaitsForJob(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	sh := New(strings.NewReader(""), &out, &errOut)

	sh.ExecuteLine("cmd /C rem done &")
	if !sh.ExecuteLine("fg 1") {
		t.Fatal("fg stopped shell")
	}
	if !strings.Contains(out.String(), "[1] done") {
		t.Fatalf("stdout = %q, want done job", out.String())
	}
	if errOut.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", errOut.String())
	}
}

func TestShellVariablesOverrideEnvironmentExpansion(t *testing.T) {
	t.Setenv("GOSH_COLOR", "env-blue")

	var out bytes.Buffer
	var errOut bytes.Buffer
	sh := New(strings.NewReader(""), &out, &errOut)

	if !sh.ExecuteLine("set GOSH_COLOR=shell-green") {
		t.Fatal("set stopped shell")
	}
	if !sh.ExecuteLine("echo $GOSH_COLOR %GOSH_COLOR%") {
		t.Fatal("echo stopped shell")
	}
	if strings.TrimSpace(out.String()) != "shell-green shell-green" {
		t.Fatalf("stdout = %q, want shell variable values", out.String())
	}
	if errOut.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", errOut.String())
	}
}

func TestShellVariablesCanBeUnset(t *testing.T) {
	t.Setenv("GOSH_MODE", "from-env")

	var out bytes.Buffer
	var errOut bytes.Buffer
	sh := New(strings.NewReader(""), &out, &errOut)

	sh.ExecuteLine("set GOSH_MODE=from-shell")
	sh.ExecuteLine("unset GOSH_MODE")
	if !sh.ExecuteLine("echo $GOSH_MODE") {
		t.Fatal("echo stopped shell")
	}
	if strings.TrimSpace(out.String()) != "from-env" {
		t.Fatalf("stdout = %q, want env fallback after unset", out.String())
	}
}

func TestScriptFunctionExpandsArguments(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	sh := New(strings.NewReader(""), &out, &errOut)

	if !sh.ExecuteLine("fn greet = echo hello $1 from $@") {
		t.Fatal("fn stopped shell")
	}
	if !sh.ExecuteLine("greet world gosh") {
		t.Fatal("function invocation stopped shell")
	}
	if strings.TrimSpace(out.String()) != "hello world from world gosh" {
		t.Fatalf("stdout = %q, want function output", out.String())
	}
	if errOut.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", errOut.String())
	}
}

func TestStartupScriptCanDefineFunctionAndVariable(t *testing.T) {
	t.Setenv("GOSH_STARTUP", "set TARGET=gosh;fn greet = echo hi $TARGET;greet")
	t.Setenv("GOSH_STARTUP_FILE", filepath.Join(t.TempDir(), "missing-startup"))

	var out bytes.Buffer
	var errOut bytes.Buffer
	sh := New(strings.NewReader("exit\n"), &out, &errOut)

	if err := sh.Run(); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !strings.Contains(out.String(), "hi gosh") {
		t.Fatalf("stdout = %q, want startup function output", out.String())
	}
	if errOut.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", errOut.String())
	}
}

func TestSnapshotRestoresVariablesFunctionsAndWorkingDirectory(t *testing.T) {
	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd returned error: %v", err)
	}

	dir := t.TempDir()
	t.Cleanup(func() {
		_ = os.Chdir(originalWD)
	})
	var out bytes.Buffer
	var errOut bytes.Buffer
	sh := New(strings.NewReader(""), &out, &errOut)

	sh.ExecuteLine("set TARGET=snapshot")
	sh.ExecuteLine("fn greet = echo hi $TARGET")
	sh.ExecuteLine("cd " + dir)
	state := sh.Snapshot()

	restored := New(strings.NewReader(""), &out, &errOut)
	if err := restored.Restore(state); err != nil {
		t.Fatalf("Restore returned error: %v", err)
	}
	out.Reset()
	if !restored.ExecuteLine("greet") {
		t.Fatal("function stopped shell")
	}
	if strings.TrimSpace(out.String()) != "hi snapshot" {
		t.Fatalf("stdout = %q, want restored function and variable", out.String())
	}
	if wd, err := os.Getwd(); err != nil || wd != dir {
		t.Fatalf("working directory = %q, %v; want %q", wd, err, dir)
	}
}

func TestStopBuiltinMarksJobStoppedWhenSupported(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	sh := New(strings.NewReader(""), &out, &errOut)

	sh.ExecuteLine("cmd /C ping 127.0.0.1 -n 2 > NUL &")
	if !sh.ExecuteLine("stop 1") {
		t.Fatal("stop stopped shell")
	}

	if executor.SupportsJobStop() {
		if !strings.Contains(out.String(), "[1] stopped") {
			t.Fatalf("stdout = %q, want stopped job", out.String())
		}
		if errOut.Len() != 0 {
			t.Fatalf("stderr = %q, want empty", errOut.String())
		}
		return
	}

	if !strings.Contains(errOut.String(), "not supported") {
		t.Fatalf("stderr = %q, want unsupported stop message", errOut.String())
	}
}
