# Lola — build and install.
#
# Everything Lola needs at runtime lives under one folder, ~/.lola (or
# $LOLA_HOME). `make install` puts the binary in ~/.lola/bin and the default
# config in ~/.lola/lola.yaml.

LOLA_HOME ?= $(HOME)/.lola

# `make` on its own installs, since that is what people almost always want.
.DEFAULT_GOAL := install

.PHONY: build test fmt vet windows check install update uninstall clean run

# Builds the binary into this directory only. It does NOT update the `lola`
# command — use `make install` for that.
build:
	go build -o lola .
	@echo "note: built ./lola here only — this did NOT update your 'lola' command. Run 'make install'."

test:
	go test ./...

# What CI runs, in one word, so a PR stops being a memory test.
check: fmt vet test windows
	@echo "fmt, vet, tests and the Windows build: all clean"

fmt:
	@out=$$(gofmt -l .); \
	if [ -n "$$out" ]; then echo "not gofmt'd:"; echo "$$out"; exit 1; fi

vet:
	go vet ./...

# The harness is meant to run on more than this machine, and Go makes that cheap
# to prove: if it cross-compiles, nothing in it has drifted into the platform.
windows:
	@GOOS=windows GOARCH=amd64 go build -o /dev/null .

# Install the binary and a default config into the Lola home directory.
install: build
	@mkdir -p $(LOLA_HOME)/bin
	@cp lola $(LOLA_HOME)/bin/lola
	@if [ ! -f $(LOLA_HOME)/lola.yaml ]; then \
		cp lola.yaml $(LOLA_HOME)/lola.yaml; \
		echo "installed default config to $(LOLA_HOME)/lola.yaml"; \
	else \
		echo "kept existing config at $(LOLA_HOME)/lola.yaml"; \
	fi
	@echo "installed $(LOLA_HOME)/bin/lola"
	@echo "add $(LOLA_HOME)/bin to your PATH to run 'lola' from anywhere"

# Update: take the latest code and replace the binary. Never the config.
# Your lola.yaml is yours — it is the thing you have been editing for weeks, and an
# upgrade that quietly rewrote it would be the worst behaviour a harness could have.
update:
	@if [ -n "$$(git status --porcelain)" ]; then \
		echo "uncommitted changes here — commit or stash them first"; exit 1; \
	fi
	git pull --ff-only
	$(MAKE) build
	@mkdir -p $(LOLA_HOME)/bin
	@cp lola $(LOLA_HOME)/bin/lola
	@echo "updated $(LOLA_HOME)/bin/lola — your config at $(LOLA_HOME)/lola.yaml is untouched"

uninstall:
	@rm -f $(LOLA_HOME)/bin/lola
	@echo "removed $(LOLA_HOME)/bin/lola"

clean:
	@rm -f lola

run: build
	./lola -frontend web -addr 127.0.0.1:8765
