# mcpshell build
#
# The ANTLR parser is generated into parser/ and committed to the repo.
# Regenerate only after editing grammar/*.g4 — requires Java 11+. The ANTLR
# tool jar is downloaded and checksum-verified by scripts/antlr.sh.

.PHONY: generate build cli install bench test vet fmt release

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

# vet skips parser/: ANTLR ends every rule with a dead `goto errorExit` so the
# label is always used, and vet reports each one as unreachable code. The
# file is generated (DO NOT EDIT), so those reports are not actionable.
vet:
	go vet $$(go list ./... | grep -v /parser$$)

fmt:
	gofmt -w .

# Release tarballs in $(DIST): mcpshell_<os>_<arch>.tar.gz with the binary
# at the archive root, plus SHA256SUMS (the shape dun's deps.json downloads).
# Build from a clean checkout of the release tag so --version reports the
# tag; a DIST outside the checkout keeps the tree clean. cgo is off: every
# dependency is pure Go. Upload with  gh release upload <tag> $(DIST)/*
DIST ?= dist
RELEASE_TARGETS = linux/amd64 linux/arm64 darwin/amd64 darwin/arm64
release:
	rm -rf $(DIST) && mkdir -p $(DIST)
	for t in $(RELEASE_TARGETS); do \
		os=$${t%/*}; arch=$${t#*/}; d=$(DIST)/mcpshell_$${os}_$${arch}; \
		mkdir -p $$d && \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -trimpath -o $$d/mcpshell ./cmd/mcpshell && \
		tar -C $$d --owner=0 --group=0 --numeric-owner -czf $$d.tar.gz mcpshell && \
		rm -rf $$d || exit 1; \
	done
	cd $(DIST) && sha256sum *.tar.gz > SHA256SUMS
