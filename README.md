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
- Stopped-job state with `stop`, plus `bg`/`fg` resume semantics on OSes that support process suspension.
- Foreground process signal forwarding for Ctrl+C/process interrupts.
- Browser UI for local command execution, persistent multi-session workspaces, history, quick commands, and jobs output.
- Interactive browser stream for long-running terminal programs, with a Linux PTY backend and a cross-platform pipe fallback.
- Wildcard expansion for external command arguments.
- Syntax errors with column positions and caret hints.
- Configurable aliases using `GOSH_ALIASES` or an aliases config file.
- Configurable startup scripts using `GOSH_STARTUP` or `GOSH_STARTUP_FILE`.
- Reusable shell variables with `set`, `unset`, and `vars`.
- One-line script functions with `fn`, `unfn`, and `functions`.
- Prompt status and timing placeholders with `{status}` and `{duration}`.
- Cross-platform `clear` behavior.
- Unit and integration tests for parser, built-ins, shell flow, process execution, and scripted CLI input.

## Run

```powershell
go run ./cmd/gosh
```

Run the browser UI:

```powershell
go run ./cmd/gosh-ui
```

Then open `http://127.0.0.1:8090`. The UI provides a local terminal surface backed by the same shell engine, with persistent browser workspaces, command history, quick commands, jobs output, job controls, history filtering, workspace settings, and an interactive stream panel for terminal programs.

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

Reusable shell variables override OS environment variables during command expansion without modifying the parent environment:

```text
set TARGET=gosh
echo $TARGET
unset TARGET
vars
```

One-line functions can be defined in an interactive session or startup script. Function arguments are available as `$1`, `$2`, `$@`, and `$#`:

```text
fn greet = echo hello $1 from $@
greet world gosh
functions
unfn greet
```

## Project Structure

```text
cmd/gosh          CLI entrypoint
cmd/gosh-ui       Browser UI entrypoint
internal/shell    interactive shell loop and command dispatch
internal/parser   command-line parser
internal/builtin  built-in commands
internal/executor external process runner
internal/ui       HTTP UI server and static frontend
```

## What This Project Demonstrates

- How a shell separates parsing, built-ins, and process execution.
- Why commands such as `cd` must be built into the shell process.
- How Windows commands can be executable programs or CMD built-ins.
- How to keep an interactive program running after command errors.
- How to test command parsing and shell behavior without manual terminal input.

## Current Scope

This version keeps job control intentionally small. It tracks background jobs, can stop and resume jobs where the host OS supports process suspension, forwards foreground interrupts to child process groups, and includes a browser UI for local command execution. Windows supports foreground interrupt forwarding with console control events, but stopped-job suspension/resume is reported as unsupported because Windows does not expose POSIX-style `SIGSTOP`/`SIGCONT` semantics.

The UI stores workspace metadata, transcripts, and command history in browser `localStorage`, while live shell state remains in the running `gosh-ui` server process. Interactive streaming uses Server-Sent Events plus POSTed input. Linux starts streams through a PTY so full-screen programs can detect a terminal; Windows and other OSes currently use a pipe-backed stream for long-running interactive commands.

## Roadmap

- Add native Windows ConPTY support for full-screen browser streams.
- Persist workspace metadata on the server for multi-browser continuity.
- Add authenticated remote access mode for non-local UI deployments.
- Add terminal resize propagation from the browser viewport.
