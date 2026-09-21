# The plain build has no cgo and reads mail straight from Go, the geas build runs mail through its contract

GO ?= go
GEAS ?= geas
OUT := target
CONTRACTS := contracts/mail.geas
MODULES := $(OUT)/geas-out/libmail.geas.so

DUSK ?= dusk
PLUGINS := plugins/mail/classify.dusk
ARCHIVES := $(OUT)/dusk-out/libclassify.a

PREFIX ?= $(HOME)/.local
BINDIR := $(PREFIX)/bin
CONTRACTDIR := $(PREFIX)/share/symphony/contracts

.PHONY: build geas dusk contracts plugins install test test-geas test-dusk lint run clean

# Both binaries, plain
build:
	$(GO) build -o $(OUT)/symphony ./cmd/symphony
	$(GO) build -o $(OUT)/symphonyd ./cmd/symphonyd

# The contracts compiled to modules under target/geas-out
contracts: $(MODULES)

$(OUT)/geas-out/lib%.geas.so: contracts/%.geas
	$(GEAS) build $<

# Both binaries with the geas runtime linked in, the modules copied beside them
geas: contracts
	$(GO) build -tags geas -o $(OUT)/symphony ./cmd/symphony
	$(GO) build -tags geas -o $(OUT)/symphonyd ./cmd/symphonyd
	mkdir -p $(OUT)/contracts
	cp $(MODULES) $(OUT)/contracts/

# The dusk plugin bodies archived under target/dusk-out
plugins: $(ARCHIVES)

$(OUT)/dusk-out/lib%.a: plugins/mail/%.dusk plugins/mail/glue.c
	$(DUSK) build --lib $<

# Both binaries with the geas runtime and the dusk bodies linked in
dusk: contracts plugins
	$(GO) build -tags "geas dusk" -o $(OUT)/symphony ./cmd/symphony
	$(GO) build -tags "geas dusk" -o $(OUT)/symphonyd ./cmd/symphonyd
	mkdir -p $(OUT)/contracts
	cp $(MODULES) $(OUT)/contracts/

# Copies whatever was last built into ~/.local, the binaries and the contract modules, install replaces a running binary where cp would fail
install:
	mkdir -p $(BINDIR) $(CONTRACTDIR)
	install -m 0755 $(OUT)/symphony $(OUT)/symphonyd $(BINDIR)/
	if [ -d $(OUT)/contracts ]; then install -m 0644 $(OUT)/contracts/*.so $(CONTRACTDIR)/; fi

# The plain suite plus the plugin tests
test:
	$(GO) test -race ./...
	nvim --headless -u NONE -l nvim/symphony.nvim/tests/run.lua

# The suite with the geas packages included
test-geas:
	$(GO) test -race -tags geas ./...

# The suite with the dusk body linked in, needs the archive from make plugins
test-dusk: plugins
	$(GO) test -race -tags "geas dusk" ./...

lint:
	gofmt -l .
	$(GO) vet ./...
	$(GO) vet -tags geas ./...
	golangci-lint run ./...
	golangci-lint run --build-tags geas ./...
	golangci-lint run --build-tags geas,dusk ./...

# The TUI from source, needs a real terminal
run:
	$(GO) run ./cmd/symphony

clean:
	rm -rf $(OUT)
