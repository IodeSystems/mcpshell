# mcpshell build
#
# The ANTLR parser is generated into parser/ and committed to the repo.
# Regenerate only after editing grammar/*.g4 — requires Java + the ANTLR 4.13.2
# tool jar and its deps (resolved from the local Maven repo).

ANTLR_VERSION := 4.13.2
M2            := $(HOME)/.m2/repository
A4  := $(M2)/org/antlr/antlr4/$(ANTLR_VERSION)/antlr4-$(ANTLR_VERSION).jar
A3  := $(M2)/org/antlr/antlr-runtime/3.5.3/antlr-runtime-3.5.3.jar
A4R := $(M2)/org/antlr/antlr4-runtime/$(ANTLR_VERSION)/antlr4-runtime-$(ANTLR_VERSION).jar
ST4 := $(M2)/org/antlr/ST4/4.3.4/ST4-4.3.4.jar
TL  := $(M2)/org/abego/treelayout/org.abego.treelayout.core/1.0.3/org.abego.treelayout.core-1.0.3.jar
ICU := $(M2)/com/ibm/icu/icu4j/73.2/icu4j-73.2.jar
ANTLR_CP := $(A4):$(A3):$(A4R):$(ST4):$(TL):$(ICU)

.PHONY: generate build cli bench test fmt

generate:
	java -cp "$(ANTLR_CP)" org.antlr.v4.Tool \
		-Dlanguage=Go -package parser -visitor -no-listener -Xexact-output-dir \
		-o parser grammar/McpShellLexer.g4 grammar/McpShellParser.g4
	gofmt -w parser/

build:
	go build ./...

# Build the host-arch binaries. The bin/ launchers do this automatically on
# demand; these targets are for an explicit ahead-of-time build.
cli:
	go build -o bin/mcpshell-$(shell go env GOOS)-$(shell go env GOARCH) ./cmd/mcpshell

bench:
	go build -o bin/bench-$(shell go env GOOS)-$(shell go env GOARCH) ./cmd/bench

test:
	go test ./...

fmt:
	gofmt -w .
