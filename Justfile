set dotenv-load

bump-deps:
    uv lock --upgrade

lint:
    ruff check ./src

lint-with-fix:
    ruff check --fix ./src

format:
    ruff format ./src

format-check:
    ruff format --check ./src
