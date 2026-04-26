# AGENTS.md

## Project

A fund valuation TUI tool built with Python, using AKShare to fetch fund data and Textual to build the terminal user interface.

## Environment

- Python 3.14+
- Use UV for dependency management
- Prefer `just` for common development commands

## Common Commands

```bash
uv sync
uv run funda
just lint
just lint-with-fix
just format
just format-check
just bump-deps
```

## Code Quality

- Use Ruff for linting and formatting
- Ruff config: `.ruff.toml`

## Git Workflow

- Never run `git commit`, `git push`, or other git mutations unless explicitly instructed
- If explicitly instructed to commit or push, execute directly without extra confirmation
- Commit message rules:
  - One sentence only
  - No Conventional Commit prefixes
  - Capitalize the first letter
  - Example: "Add delete menu to connection list"
