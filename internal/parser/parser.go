package parser

import (
	"errors"
	"fmt"
	"os"
	"strconv"
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
	Background     bool
	Raw            string
}

type SyntaxError struct {
	Message string
	Column  int
}

func (e *SyntaxError) Error() string {
	return fmt.Sprintf("%v at column %d: %s", ErrInvalidSyntax, e.Column, e.Message)
}

func (e *SyntaxError) Unwrap() error {
	return ErrInvalidSyntax
}

type token struct {
	value  string
	column int
}

func Parse(input string) (Command, error) {
	line, err := ParseLine(input)
	if err != nil {
		return Command{}, err
	}
	if len(line.Commands) == 0 {
		return Command{}, nil
	}
	if len(line.Commands) > 1 || line.InputRedirect != "" || line.OutputRedirect != "" || line.Background {
		return Command{}, fmt.Errorf("%w: expected a single command", ErrInvalidSyntax)
	}
	return line.Commands[0], nil
}

func ParseLine(input string) (Line, error) {
	return ParseLineWithEnv(input, lookupOSEnv)
}

func ParseLineWithEnv(input string, lookup func(string) (string, bool)) (Line, error) {
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
	var current []token

	flushCommand := func() error {
		if len(current) == 0 {
			return syntaxError("empty command", 1)
		}
		values := tokenValues(current)
		expanded := expandTokens(values, lookup)
		line.Commands = append(line.Commands, Command{
			Name: expanded[0],
			Args: expanded[1:],
			Raw:  strings.Join(values, " "),
		})
		current = nil
		return nil
	}

	for i := 0; i < len(tokens); i++ {
		token := tokens[i]
		switch token.value {
		case "|":
			if err := flushCommand(); err != nil {
				return Line{}, err
			}
		case "<":
			i++
			if i >= len(tokens) || isOperator(tokens[i].value) {
				return Line{}, syntaxError("missing input file", token.column)
			}
			line.InputRedirect = expandToken(tokens[i].value, lookup)
		case ">", ">>":
			i++
			if i >= len(tokens) || isOperator(tokens[i].value) {
				return Line{}, syntaxError("missing output file", token.column)
			}
			line.OutputRedirect = expandToken(tokens[i].value, lookup)
			line.AppendOutput = token.value == ">>"
		case "&":
			if i != len(tokens)-1 {
				return Line{}, syntaxError("background marker must be at the end", token.column)
			}
			line.Background = true
		default:
			current = append(current, token)
		}
	}

	if err := flushCommand(); err != nil {
		return Line{}, err
	}
	return line, nil
}

func tokenize(input string) ([]token, error) {
	var tokens []token
	var current strings.Builder
	currentColumn := 0
	quote := rune(0)
	escaping := false
	tokenStarted := false

	runes := []rune(input)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		column := i + 1
		switch {
		case escaping:
			current.WriteString(escapeSequence(r, quote))
			escaping = false
		case r == '\\' && quote != '\'':
			if currentColumn == 0 {
				currentColumn = column
			}
			tokenStarted = true
			escaping = true
		case r == '"' || r == '\'':
			if quote == 0 {
				if currentColumn == 0 {
					currentColumn = column
				}
				tokenStarted = true
				quote = r
				continue
			}
			if quote == r {
				quote = 0
				continue
			}
			current.WriteRune(r)
		case quote == 0 && (r == '|' || r == '<' || r == '>' || r == '&'):
			if tokenStarted {
				tokens = append(tokens, token{value: current.String(), column: currentColumn})
				current.Reset()
				currentColumn = 0
				tokenStarted = false
			}
			if r == '>' && i+1 < len(runes) && runes[i+1] == '>' {
				tokens = append(tokens, token{value: ">>", column: column})
				i++
			} else {
				tokens = append(tokens, token{value: string(r), column: column})
			}
		case isSpace(r) && quote == 0:
			if tokenStarted {
				tokens = append(tokens, token{value: current.String(), column: currentColumn})
				current.Reset()
				currentColumn = 0
				tokenStarted = false
			}
		default:
			if currentColumn == 0 {
				currentColumn = column
			}
			tokenStarted = true
			current.WriteRune(r)
		}
	}

	if escaping {
		current.WriteRune('\\')
	}
	if quote != 0 {
		return nil, syntaxError("unclosed quote", currentColumn)
	}
	if tokenStarted {
		tokens = append(tokens, token{value: current.String(), column: currentColumn})
	}

	return tokens, nil
}

func isSpace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\r' || r == '\n'
}

func escapeSequence(r rune, quote rune) string {
	if quote == '"' {
		switch r {
		case 'n':
			return "\n"
		case 'r':
			return "\r"
		case 't':
			return "\t"
		default:
			return string(r)
		}
	}

	if quote == 0 && (isSpace(r) || strings.ContainsRune(`|<>&"'\`, r)) {
		return string(r)
	}
	return `\` + string(r)
}

func isOperator(token string) bool {
	return token == "|" || token == "<" || token == ">" || token == ">>" || token == "&"
}

func syntaxError(message string, column int) error {
	if column < 1 {
		column = 1
	}
	if message == "unclosed quote" {
		return fmt.Errorf("%w: %w", ErrUnclosedQuote, &SyntaxError{Message: message, Column: column})
	}
	return &SyntaxError{Message: message, Column: column}
}

func tokenValues(tokens []token) []string {
	values := make([]string, len(tokens))
	for i, token := range tokens {
		values[i] = token.value
	}
	return values
}

func expandTokens(tokens []string, lookup func(string) (string, bool)) []string {
	expanded := make([]string, len(tokens))
	for i, token := range tokens {
		expanded[i] = expandToken(token, lookup)
	}
	return expanded
}

func expandToken(token string, lookup func(string) (string, bool)) string {
	return expandWindowsEnv(os.Expand(token, func(name string) string {
		value, _ := lookup(name)
		return value
	}), lookup)
}

func expandWindowsEnv(token string, lookup func(string) (string, bool)) string {
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
		value, _ := lookup(name)
		out.WriteString(value)
		i += end + 1
	}
	return out.String()
}

func lookupOSEnv(name string) (string, bool) {
	return os.LookupEnv(name)
}

func FormatError(input string, err error) string {
	var syntaxErr *SyntaxError
	if !errors.As(err, &syntaxErr) {
		return err.Error()
	}

	column := syntaxErr.Column
	if column < 1 {
		column = 1
	}

	var b strings.Builder
	b.WriteString(syntaxErr.Error())
	b.WriteString("\n")
	b.WriteString(input)
	b.WriteString("\n")
	b.WriteString(strings.Repeat(" ", column-1))
	b.WriteString("^")
	return b.String()
}

func QuoteForRaw(values []string) string {
	quoted := make([]string, len(values))
	for i, value := range values {
		if strings.ContainsAny(value, " \t|<>&\"") {
			quoted[i] = strconv.Quote(value)
		} else {
			quoted[i] = value
		}
	}
	return strings.Join(quoted, " ")
}
