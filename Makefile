# mcpshell build
#
# The ANTLR parser is generated into parser/ and committed to the repo.
# Regenerate only after editing grammar/*.g4 — requires Java 11+. The ANTLR
# tool jar is downloaded and checksum-verified by scripts/antlr.sh.

.PHONY: generate build cli install bench test fmt

PREFIX ?= $(HOME)/.local

generate:
	./scripts/antlr.sh \
		-Dlanguage=Go -package parser -visitor -no-listener -Xexact-output-dir \
		-o parser grammar/McpShellLexer.g4 grammar/McpShellParser.g4
	gofmt -w parser/

build:
	go build ./...

# Build the host-arch binaries. The bin/ launchers do this automatically on
# demand; these targets are for an explicit ahead-of-time build.
cli:
	go build -o bin/mcpshell-$(shell go env GOOS)-$(shell go env GOARCH) ./cmd/mcpshell

# Install mcpshell to $(PREFIX)/bin (default ~/.local/bin).
install:
	GOBIN=$(PREFIX)/bin go install ./cmd/mcpshell

bench:
	go build -o bin/bench-$(shell go env GOOS)-$(shell go env GOARCH) ./cmd/bench

test:
	go test ./...

fmt:
	gofmt -w .
