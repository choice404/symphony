# The plain build has no cgo and reads mail straight from Go, the geas build runs mail through its contract

GO ?= go
GEAS ?= geas
OUT := target
CONTRACTS := contracts/mail.geas
MODULES := $(OUT)/geas-out/libmail.geas.so

.PHONY: build geas contracts test test-geas lint run clean

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

# The plain suite plus the plugin tests
test:
	$(GO) test -race ./...
	nvim --headless -u NONE -l nvim/symphony.nvim/tests/run.lua

# The suite with the geas packages included
test-geas:
	$(GO) test -race -tags geas ./...

lint:
	gofmt -l .
	$(GO) vet ./...
	$(GO) vet -tags geas ./...
	golangci-lint run ./...
	golangci-lint run --build-tags geas ./...

# The TUI from source, needs a real terminal
run:
	$(GO) run ./cmd/symphony

clean:
	rm -rf $(OUT)
