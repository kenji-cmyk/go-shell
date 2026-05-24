package parser

import (
	"errors"
	"strings"
)

var ErrUnclosedQuote = errors.New("unclosed quote")

type Command struct {
	Name string
	Args []string
	Raw  string
}

func Parse(input string) (Command, error) {
	raw := strings.TrimSpace(input)
	if raw == "" {
		return Command{}, nil
	}

	tokens, err := tokenize(raw)
	if err != nil {
		return Command{}, err
	}
	if len(tokens) == 0 {
		return Command{}, nil
	}

	return Command{
		Name: tokens[0],
		Args: tokens[1:],
		Raw:  raw,
	}, nil
}

func tokenize(input string) ([]string, error) {
	var tokens []string
	var current strings.Builder
	inQuote := false
	escaping := false

	for _, r := range input {
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
