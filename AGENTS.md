# AGENTS.md

## Project Background

A fund valuation TUI tool built with Python, using AKShare to fetch fund data and Textual to build the terminal user interface.

## Development Environment

- **Python**: 3.14+
- **Package Manager**: [UV](https://github.com/astral-sh/uv)
- **Linting**: [Ruff](https://github.com/astral-sh/ruff)

## Common Commands

```bash
# Install dependencies
uv sync

# Run main program
uv run funda

# Run linter
uv run ruff check src/

# Run formatter
uv run ruff format src/
```

## Code Style

- Use **Ruff** for linting and formatting
- Configuration in `.ruff.toml`

## Dependency Updates

- Use **Dependabot** for automated dependency updates
- Configuration in `.github/dependabot.yml`
- Supports UV ecosystem
- Checks for updates weekly on Mondays

## Notes

1. **config.yaml** contains user-sensitive information and is added to `.gitignore`. Do not commit it to the repository.
2. Use `uv.lock` to lock dependency versions and ensure environment consistency.
3. Ensure code passes Ruff checks before committing.
