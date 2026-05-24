# Go Shell

Go Shell (`gosh`) is a small Windows-friendly shell written in Go. The goal is not to replace CMD or PowerShell, but to show the core pieces that make a shell work: reading commands, parsing arguments, handling built-ins, launching processes, and reporting errors without crashing the session.

## Features

- Interactive read-eval loop with a current-directory prompt.
- Command history, arrow-key navigation, and tab completion in interactive mode.
- Parser for whitespace, single and double quotes, escaped spaces, escaped operators, and empty quoted arguments.
- Built-in commands: `cd`, `pwd`, `exit`, `help`, `echo`, `clear`, and `cls`.
- External command execution with forwarded `stdin`, `stdout`, and `stderr`.
- CMD fallback for common Windows shell commands: `dir`, `cls`, `copy`, `del`, and `type`.
- Pipelines with `|`.
- Input and output redirects with `<`, `>`, and `>>`.
- Environment variable expansion with `$NAME` and `%NAME%`.
- Configurable prompt using the `GOSH_PROMPT` environment variable.
- Background jobs with `&`.
- Job listing and foreground/background commands with `jobs`, `fg`, and `bg`.
- Wildcard expansion for external command arguments.
- Syntax errors with column positions and caret hints.
- Configurable aliases using `GOSH_ALIASES` or an aliases config file.
- Configurable startup scripts using `GOSH_STARTUP` or `GOSH_STARTUP_FILE`.
- Prompt status and timing placeholders with `{status}` and `{duration}`.
- Cross-platform `clear` behavior.
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
projects> notepad &
projects> jobs
projects> fg 1
projects> go version
projects> exit
```

Prompt templates can use `{base}`, `{cwd}`, `{status}`, and `{duration}`:

```powershell
$env:GOSH_PROMPT = "[{base} {status} {duration}]$ "
go run ./cmd/gosh
```

Aliases can be configured inline:

```powershell
$env:GOSH_ALIASES = "ll=dir;gs=git status"
go run ./cmd/gosh
```

Or with a config file at the default user config path for `gosh`, one `name=command` pair per line:

```text
ll=dir
gs=git status
```

Startup commands can be configured inline or through a file:

```powershell
$env:GOSH_STARTUP = "echo welcome;pwd"
$env:GOSH_STARTUP_FILE = "C:\Users\you\.config\gosh\startup.gosh"
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

This version intentionally keeps job control small. It can list jobs and wait on background jobs with `fg`, while `bg` reports already-running jobs; it does not yet include stopped-job resume semantics, signal forwarding, or PowerShell-style object pipelines.

## Roadmap

- Add stopped-job support and real process resume semantics where the OS allows it.
- Add signal forwarding and Ctrl+C process-group handling.
- Add script functions and reusable shell variables.
- Add a small integration test harness for interactive terminal behavior.

