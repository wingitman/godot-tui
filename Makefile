BINARY := godot-tui
BUILD_DIR := bin
RELEASES_DIR := releases
INSTALL_DIR ?= $(HOME)/.local/bin
MAN_DIR ?= $(HOME)/.local/share/man/man1
COMMIT := $(shell git rev-parse --verify HEAD 2>/dev/null || printf dev)
LDFLAGS := -s -w -X github.com/wingitman/godot-tui/internal/version.Commit=$(COMMIT)

.PHONY: all build build-all install uninstall test test-all clean
all: build
build:
	@mkdir -p $(BUILD_DIR)
	go build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY) .
build-all:
	@mkdir -p $(RELEASES_DIR)/linux/amd64 $(RELEASES_DIR)/linux/arm64 $(RELEASES_DIR)/darwin/amd64 $(RELEASES_DIR)/darwin/arm64 $(RELEASES_DIR)/windows
	GOOS=linux GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o $(RELEASES_DIR)/linux/amd64/$(BINARY) .
	GOOS=linux GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o $(RELEASES_DIR)/linux/arm64/$(BINARY) .
	GOOS=darwin GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o $(RELEASES_DIR)/darwin/amd64/$(BINARY) .
	GOOS=darwin GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o $(RELEASES_DIR)/darwin/arm64/$(BINARY) .
	GOOS=windows GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o $(RELEASES_DIR)/windows/$(BINARY).exe .
install: build install-man
	@mkdir -p $(INSTALL_DIR)
	cp $(BUILD_DIR)/$(BINARY) $(INSTALL_DIR)/$(BINARY)
	@chmod +x $(INSTALL_DIR)/$(BINARY)
	@echo "Installed $(INSTALL_DIR)/$(BINARY)"
install-man:
	@mkdir -p $(MAN_DIR)
	cp docs/godot-tui.1 $(MAN_DIR)/godot-tui.1
	@echo "Installed $(MAN_DIR)/godot-tui.1"
uninstall:
	rm -f $(INSTALL_DIR)/$(BINARY)
	rm -f $(MAN_DIR)/godot-tui.1
test:
	go test ./...
test-all: test
clean:
	rm -rf $(BUILD_DIR)
