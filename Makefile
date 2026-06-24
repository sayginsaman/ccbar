BIN := ccbar
INSTALL_DIR := $(HOME)/.claude/ccbar

.PHONY: all build test vet fmt demo install uninstall clean

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

install:
	./install.sh

uninstall:
	./uninstall.sh

clean:
	rm -f $(BIN) $(BIN).new
