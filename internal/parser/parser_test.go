package parser

import (
	"errors"
	"strings"
	"testing"
)

func TestParseSimpleCommand(t *testing.T) {
	cmd, err := Parse("echo hello")
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if cmd.Name != "echo" {
		t.Fatalf("Name = %q, want echo", cmd.Name)
	}
	if len(cmd.Args) != 1 || cmd.Args[0] != "hello" {
		t.Fatalf("Args = %#v, want [hello]", cmd.Args)
	}
}

func TestParseCollapsesExtraSpaces(t *testing.T) {
	cmd, err := Parse("echo    hello    world")
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	want := []string{"hello", "world"}
	if len(cmd.Args) != len(want) {
		t.Fatalf("Args = %#v, want %#v", cmd.Args, want)
	}
	for i := range want {
		if cmd.Args[i] != want[i] {
			t.Fatalf("Args = %#v, want %#v", cmd.Args, want)
		}
	}
}

func TestParseQuotedArgument(t *testing.T) {
	cmd, err := Parse(`echo "hello world"`)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if len(cmd.Args) != 1 || cmd.Args[0] != "hello world" {
		t.Fatalf("Args = %#v, want [hello world]", cmd.Args)
	}
}

func TestParseEscapedQuote(t *testing.T) {
	cmd, err := Parse(`echo "hello \"world\""`)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if len(cmd.Args) != 1 || cmd.Args[0] != `hello "world"` {
		t.Fatalf("Args = %#v, want escaped quote", cmd.Args)
	}
}

func TestParseEmptyInput(t *testing.T) {
	cmd, err := Parse("   ")
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if cmd.Name != "" || len(cmd.Args) != 0 {
		t.Fatalf("Command = %#v, want zero command", cmd)
	}
}

func TestParseUnclosedQuote(t *testing.T) {
	_, err := Parse(`echo "hello`)
	if !errors.Is(err, ErrUnclosedQuote) {
		t.Fatalf("error = %v, want ErrUnclosedQuote", err)
	}
}

func TestParseLinePipeline(t *testing.T) {
	line, err := ParseLine(`cmd /C echo hello | findstr hello`)
	if err != nil {
		t.Fatalf("ParseLine returned error: %v", err)
	}

	if len(line.Commands) != 2 {
		t.Fatalf("command count = %d, want 2", len(line.Commands))
	}
	if line.Commands[0].Name != "cmd" {
		t.Fatalf("first command = %q, want cmd", line.Commands[0].Name)
	}
	if line.Commands[1].Name != "findstr" {
		t.Fatalf("second command = %q, want findstr", line.Commands[1].Name)
	}
}

func TestParseLineRedirects(t *testing.T) {
	line, err := ParseLine(`findstr hello < input.txt >> output.txt`)
	if err != nil {
		t.Fatalf("ParseLine returned error: %v", err)
	}

	if line.InputRedirect != "input.txt" {
		t.Fatalf("InputRedirect = %q, want input.txt", line.InputRedirect)
	}
	if line.OutputRedirect != "output.txt" {
		t.Fatalf("OutputRedirect = %q, want output.txt", line.OutputRedirect)
	}
	if !line.AppendOutput {
		t.Fatal("AppendOutput = false, want true")
	}
}

func TestParseLineExpandsEnvironmentVariables(t *testing.T) {
	t.Setenv("GOSH_TEST_VALUE", "expanded")

	line, err := ParseLine(`echo %GOSH_TEST_VALUE% $GOSH_TEST_VALUE`)
	if err != nil {
		t.Fatalf("ParseLine returned error: %v", err)
	}

	if got := line.Commands[0].Args; len(got) != 2 || got[0] != "expanded" || got[1] != "expanded" {
		t.Fatalf("Args = %#v, want expanded values", got)
	}
}

func TestParseLineBackground(t *testing.T) {
	line, err := ParseLine(`notepad &`)
	if err != nil {
		t.Fatalf("ParseLine returned error: %v", err)
	}

	if !line.Background {
		t.Fatal("Background = false, want true")
	}
	if len(line.Commands) != 1 || line.Commands[0].Name != "notepad" {
		t.Fatalf("Commands = %#v, want notepad", line.Commands)
	}
}

func TestParseLineBackgroundMustBeAtEnd(t *testing.T) {
	_, err := ParseLine(`echo one & echo two`)
	if !errors.Is(err, ErrInvalidSyntax) {
		t.Fatalf("error = %v, want ErrInvalidSyntax", err)
	}

	var syntaxErr *SyntaxError
	if !errors.As(err, &syntaxErr) {
		t.Fatalf("error = %T, want SyntaxError", err)
	}
	if syntaxErr.Column != 10 {
		t.Fatalf("column = %d, want 10", syntaxErr.Column)
	}
}

func TestFormatErrorShowsCaret(t *testing.T) {
	input := `echo "hello`
	_, err := ParseLine(input)
	if err == nil {
		t.Fatal("ParseLine returned nil error")
	}

	formatted := FormatError(input, err)
	if !strings.Contains(formatted, "column 6") || !strings.Contains(formatted, "^") {
		t.Fatalf("formatted error = %q, want column and caret", formatted)
	}
}
