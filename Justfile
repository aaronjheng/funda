set dotenv-load

bump-deps:
    go get -u ./...
    go mod tidy

lint:
    golangci-lint run --allow-parallel-runners ./...

lint-with-fix:
    golangci-lint run --allow-parallel-runners --fix ./...

install:
    go install github.com/aaronjheng/funda/cmd/funda@$(git ls-remote https://github.com/aaronjheng/funda.git refs/heads/main | cut -f1)
