.PHONY: test build run migrate integration check
GO ?= go
build:
	$(GO) build -o bin/api ./cmd/api
run:
	$(GO) run ./cmd/api
migrate:
	$(GO) run ./cmd/migrate
check:
	$(GO) vet ./...
	$(GO) test ./...
test:
	$(GO) test ./...
integration:
	@test -n "$$TEST_DATABASE_URL" || (echo 'Set TEST_DATABASE_URL to an isolated migrated database'; exit 1)
	$(GO) test -race -count=1 ./internal/api
