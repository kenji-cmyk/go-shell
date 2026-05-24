package parser

import (
	"errors"
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
