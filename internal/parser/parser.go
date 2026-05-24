package parser

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

var ErrUnclosedQuote = errors.New("unclosed quote")
var ErrInvalidSyntax = errors.New("invalid syntax")

type Command struct {
	Name string
	Args []string
	Raw  string
}

type Line struct {
	Commands       []Command
	InputRedirect  string
	OutputRedirect string
	AppendOutput   bool
	Raw            string
}

func Parse(input string) (Command, error) {
	line, err := ParseLine(input)
	if err != nil {
		return Command{}, err
	}
	if len(line.Commands) == 0 {
		return Command{}, nil
	}
	if len(line.Commands) > 1 || line.InputRedirect != "" || line.OutputRedirect != "" {
		return Command{}, fmt.Errorf("%w: expected a single command", ErrInvalidSyntax)
	}
	return line.Commands[0], nil
}

func ParseLine(input string) (Line, error) {
	raw := strings.TrimSpace(input)
	if raw == "" {
		return Line{}, nil
	}

	tokens, err := tokenize(raw)
	if err != nil {
		return Line{}, err
	}
	if len(tokens) == 0 {
		return Line{}, nil
	}

	line := Line{Raw: raw}
	var current []string

	flushCommand := func() error {
		if len(current) == 0 {
			return fmt.Errorf("%w: empty command", ErrInvalidSyntax)
		}
		expanded := expandTokens(current)
		line.Commands = append(line.Commands, Command{
			Name: expanded[0],
			Args: expanded[1:],
			Raw:  strings.Join(current, " "),
		})
		current = nil
		return nil
	}

	for i := 0; i < len(tokens); i++ {
		token := tokens[i]
		switch token {
		case "|":
			if err := flushCommand(); err != nil {
				return Line{}, err
			}
		case "<":
			i++
			if i >= len(tokens) || isOperator(tokens[i]) {
				return Line{}, fmt.Errorf("%w: missing input file", ErrInvalidSyntax)
			}
			line.InputRedirect = expandToken(tokens[i])
		case ">", ">>":
			i++
			if i >= len(tokens) || isOperator(tokens[i]) {
				return Line{}, fmt.Errorf("%w: missing output file", ErrInvalidSyntax)
			}
			line.OutputRedirect = expandToken(tokens[i])
			line.AppendOutput = token == ">>"
		default:
			current = append(current, token)
		}
	}

	if err := flushCommand(); err != nil {
		return Line{}, err
	}
	return line, nil
}

func tokenize(input string) ([]string, error) {
	var tokens []string
	var current strings.Builder
	inQuote := false
	escaping := false

	runes := []rune(input)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch {
		case escaping:
			if inQuote && (r == '"' || r == '\\') {
				current.WriteRune(r)
			} else {
				current.WriteRune('\\')
				current.WriteRune(r)
			}
			escaping = false
		case r == '\\' && inQuote:
			escaping = true
		case r == '"':
			inQuote = !inQuote
		case !inQuote && (r == '|' || r == '<' || r == '>'):
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
			if r == '>' && i+1 < len(runes) && runes[i+1] == '>' {
				tokens = append(tokens, ">>")
				i++
			} else {
				tokens = append(tokens, string(r))
			}
		case isSpace(r) && !inQuote:
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(r)
		}
	}

	if escaping {
		current.WriteRune('\\')
	}
	if inQuote {
		return nil, ErrUnclosedQuote
	}
	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}

	return tokens, nil
}

func isSpace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\r' || r == '\n'
}

func isOperator(token string) bool {
	return token == "|" || token == "<" || token == ">" || token == ">>"
}

func expandTokens(tokens []string) []string {
	expanded := make([]string, len(tokens))
	for i, token := range tokens {
		expanded[i] = expandToken(token)
	}
	return expanded
}

func expandToken(token string) string {
	return expandWindowsEnv(os.ExpandEnv(token))
}

func expandWindowsEnv(token string) string {
	var out strings.Builder
	for i := 0; i < len(token); i++ {
		if token[i] != '%' {
			out.WriteByte(token[i])
			continue
		}

		end := strings.IndexByte(token[i+1:], '%')
		if end < 0 {
			out.WriteByte(token[i])
			continue
		}

		name := token[i+1 : i+1+end]
		out.WriteString(os.Getenv(name))
		i += end + 1
	}
	return out.String()
}
