# Go Shell

Go Shell (`gosh`) is a small Windows-friendly shell written in Go. The goal is not to replace CMD or PowerShell, but to show the core pieces that make a shell work: reading commands, parsing arguments, handling built-ins, launching processes, and reporting errors without crashing the session.

## Features

- Interactive read-eval loop with a current-directory prompt.
- Command history, arrow-key navigation, and tab completion in interactive mode.
- Parser for whitespace, double-quoted arguments, and escaped quotes.
- Built-in commands: `cd`, `pwd`, `exit`, `help`, `echo`, `clear`, and `cls`.
- External command execution with forwarded `stdin`, `stdout`, and `stderr`.
- CMD fallback for common Windows shell commands: `dir`, `cls`, `copy`, `del`, and `type`.
- Pipelines with `|`.
- Input and output redirects with `<`, `>`, and `>>`.
- Environment variable expansion with `$NAME` and `%NAME%`.
- Configurable prompt using the `GOSH_PROMPT` environment variable.
- Unit tests for parser, built-ins, shell flow, and process execution.

## Run

```powershell
go run ./cmd/gosh
```

Example session:

```text
go-shell> help
go-shell> pwd
go-shell> cd ..
projects> dir
projects> echo "hello world"
projects> echo "hello" > hello.txt
projects> type hello.txt | findstr hello
projects> go version
projects> exit
```

Prompt templates can use `{base}` for the current directory name and `{cwd}` for the full current directory:

```powershell
$env:GOSH_PROMPT = "[{base}]$ "
go run ./cmd/gosh
```

## Project Structure

```text
cmd/gosh          CLI entrypoint
internal/shell    interactive shell loop and command dispatch
internal/parser   command-line parser
internal/builtin  built-in commands
internal/executor external process runner
```

## What This Project Demonstrates

- How a shell separates parsing, built-ins, and process execution.
- Why commands such as `cd` must be built into the shell process.
- How Windows commands can be executable programs or CMD built-ins.
- How to keep an interactive program running after command errors.
- How to test command parsing and shell behavior without manual terminal input.

## Current Scope

This version intentionally keeps job control small. It does not support background jobs, wildcard expansion, advanced quoting rules, or PowerShell-style object pipelines yet.



