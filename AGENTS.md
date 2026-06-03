# AGENTS.md

## Project

A fund valuation TUI tool built with Go, using Bubble Tea for the terminal user interface and fetching fund data from EastMoney APIs.

## Environment

- Go 1.26+
- Use standard `go` toolchain for dependency management
- Prefer `just` for common development commands

## Common Commands

```bash
just lint
just lint-with-fix
just bump-deps
```

## Code Quality

- Use golangci-lint for linting and formatting
- golangci-lint config: `.golangci.yaml`

## Testing

- Do NOT create unit tests proactively
- Do NOT run tests proactively
- Only create or run tests when explicitly requested by the user

## Git Workflow

- Never run `git commit`, `git push`, or other git mutations unless explicitly instructed
- If explicitly instructed to commit or push, execute directly without extra confirmation
- Commit message rules:
  - One sentence only
  - No Conventional Commit prefixes
  - Capitalize the first letter
  - Example: "Add delete menu to connection list"
