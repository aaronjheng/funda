# AGENTS.md

## Project

A fund valuation TUI tool built with Python, using AKShare to fetch fund data and Textual to build the terminal user interface.

## Environment

- Python 3.14+
- Use UV for dependency management and running commands

## Common Commands

```bash
uv sync
uv run funda
uv run ruff check src/
uv run ruff format src/
```

## Code Quality

- Use Ruff for linting and formatting
- Ruff config: `.ruff.toml`

## Git Workflow

- Never run `git commit`, `git push`, or other git mutations unless explicitly instructed
- Always ask for confirmation before committing or pushing
- Commit message rules:
  - One sentence only
  - No Conventional Commit prefixes
  - Capitalize the first letter
  - Example: "Add delete menu to connection list"
