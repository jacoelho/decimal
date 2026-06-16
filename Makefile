.SUFFIXES:
MAKEFLAGS += -r -R

BIN := $(CURDIR)/bin
GOLANGCI_LINT_VERSION := v2.12.2
export GOBIN := $(BIN)

.PHONY: test
test:
	go test ./...

.PHONY: race
race:
	go test -race ./...

.PHONY: lint
lint: $(BIN)/golangci-lint
	$(BIN)/golangci-lint run

$(BIN)/golangci-lint: go.mod .golangci.yml Makefile | $(BIN)
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

.PHONY: bench-smoke
bench-smoke:
	go test -run '^$$' -bench=. -benchtime=100ms -benchmem ./...

$(BIN):
	mkdir -p $@
