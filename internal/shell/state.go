package shell

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"go-shell/internal/parser"
)

func (s *Shell) lookupEnv(name string) (string, bool) {
	if value, ok := s.variables[name]; ok {
		return value, true
	}
	return os.LookupEnv(name)
}

func (s *Shell) runStateCommand(cmd parser.Command) (bool, bool) {
	switch strings.ToLower(cmd.Name) {
	case "set":
		return true, s.setVariable(cmd)
	case "unset":
		return true, s.unsetVariable(cmd)
	case "vars":
		s.printVariables()
		return true, true
	case "fn":
		return true, s.defineFunction(cmd)
	case "unfn":
		return true, s.removeFunction(cmd)
	case "functions":
		s.printFunctions()
		return true, true
	default:
		return false, true
	}
}

func (s *Shell) setVariable(cmd parser.Command) bool {
	if len(cmd.Args) == 0 {
		s.printVariables()
		return true
	}
	if len(cmd.Args) != 1 {
		fmt.Fprintln(s.err, "set: usage: set NAME=value")
		return false
	}
	name, value, ok := strings.Cut(cmd.Args[0], "=")
	if !ok || !isShellName(name) {
		fmt.Fprintln(s.err, "set: usage: set NAME=value")
		return false
	}
	s.variables[name] = value
	return true
}

func (s *Shell) unsetVariable(cmd parser.Command) bool {
	if len(cmd.Args) != 1 || !isShellName(cmd.Args[0]) {
		fmt.Fprintln(s.err, "unset: usage: unset NAME")
		return false
	}
	delete(s.variables, cmd.Args[0])
	return true
}

func (s *Shell) printVariables() {
	names := make([]string, 0, len(s.variables))
	for name := range s.variables {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		fmt.Fprintf(s.out, "%s=%s\n", name, s.variables[name])
	}
}

func (s *Shell) defineFunction(cmd parser.Command) bool {
	name, body, ok := parseFunctionDefinition(cmd.Raw)
	if !ok {
		fmt.Fprintln(s.err, "fn: usage: fn NAME = command")
		return false
	}
	s.functions[strings.ToLower(name)] = body
	return true
}

func (s *Shell) removeFunction(cmd parser.Command) bool {
	if len(cmd.Args) != 1 || !isShellName(cmd.Args[0]) {
		fmt.Fprintln(s.err, "unfn: usage: unfn NAME")
		return false
	}
	delete(s.functions, strings.ToLower(cmd.Args[0]))
	return true
}

func (s *Shell) printFunctions() {
	names := make([]string, 0, len(s.functions))
	for name := range s.functions {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		fmt.Fprintf(s.out, "%s=%s\n", name, s.functions[name])
	}
}

func (s *Shell) runFunction(cmd parser.Command) (bool, bool) {
	body, ok := s.functions[strings.ToLower(cmd.Name)]
	if !ok {
		return false, true
	}
	if s.functionDepth >= 16 {
		fmt.Fprintf(s.err, "%s: function recursion limit exceeded\n", cmd.Name)
		return true, false
	}

	s.functionDepth++
	defer func() {
		s.functionDepth--
	}()
	return true, s.ExecuteLine(expandFunctionBody(body, cmd.Args))
}

func parseFunctionDefinition(raw string) (string, string, bool) {
	fields := strings.Fields(raw)
	if len(fields) < 4 || !strings.EqualFold(fields[0], "fn") {
		return "", "", false
	}
	name := fields[1]
	if !isShellName(name) {
		return "", "", false
	}

	equals := strings.Index(raw, "=")
	if equals < 0 {
		return "", "", false
	}
	before := strings.TrimSpace(raw[:equals])
	if len(strings.Fields(before)) != 2 {
		return "", "", false
	}
	body := strings.TrimSpace(raw[equals+1:])
	if body == "" {
		return "", "", false
	}
	return name, body, true
}

func isShellName(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if r == '_' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' {
			continue
		}
		if i > 0 && r >= '0' && r <= '9' {
			continue
		}
		return false
	}
	return true
}

func expandFunctionBody(body string, args []string) string {
	replacements := map[string]string{
		"$#": strconv.Itoa(len(args)),
		"$@": parser.QuoteForRaw(args),
	}
	for i, arg := range args {
		replacements["$"+strconv.Itoa(i+1)] = parser.QuoteForRaw([]string{arg})
	}

	var out strings.Builder
	for i := 0; i < len(body); i++ {
		if body[i] != '$' {
			out.WriteByte(body[i])
			continue
		}
		if i+1 >= len(body) {
			out.WriteByte(body[i])
			continue
		}
		next := body[i+1]
		if next == '@' || next == '#' {
			out.WriteString(replacements[body[i:i+2]])
			i++
			continue
		}
		if next < '0' || next > '9' {
			out.WriteByte(body[i])
			continue
		}

		j := i + 1
		for j < len(body) && body[j] >= '0' && body[j] <= '9' {
			j++
		}
		if value, ok := replacements[body[i:j]]; ok {
			out.WriteString(value)
		}
		i = j - 1
	}
	return out.String()
}
