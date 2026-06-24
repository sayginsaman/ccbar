BIN := ccbar
INSTALL_DIR := $(HOME)/.claude/ccbar

.PHONY: all build test vet fmt demo install uninstall release-snapshot clean

all: vet test build

build:
	CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o $(BIN) .

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -l -w .

demo: build
	COLUMNS=$${COLUMNS:-120} ./$(BIN) --demo

# Build from source and register as the Claude Code status line.
install: build
	./$(BIN) install

uninstall: build
	./$(BIN) uninstall

# Dry-run the cross-platform release locally (no publish); needs goreleaser.
release-snapshot:
	goreleaser release --snapshot --clean

clean:
	rm -f $(BIN) $(BIN).new
	rm -rf dist
