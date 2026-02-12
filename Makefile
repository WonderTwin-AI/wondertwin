.PHONY: build build-twins build-all clean test vet release-local

VERSION ?= dev
LDFLAGS := -ldflags "-s -w -X main.version=$(VERSION)"

TWINS := stripe twilio resend posthog clerk logodev

build: ## Build the wt CLI binary
	go build $(LDFLAGS) -o bin/wt ./cmd/wt/
	@echo "Built bin/wt (version=$(VERSION))"

build-twins: ## Build all twin binaries
	@mkdir -p bin
	$(foreach twin,$(TWINS),go build -o bin/twin-$(twin) ./twin-$(twin)/cmd/twin-$(twin)/;)
	@echo "Built twins: $(TWINS)"

build-all: build build-twins ## Build wt CLI and all twins

clean: ## Remove build artifacts
	rm -rf bin/ dist/

test: ## Run all tests
	go test ./... -count=1

vet: ## Run go vet
	go vet ./...

release-local: ## Cross-compile for all supported platforms into dist/
	@mkdir -p dist
	GOOS=darwin  GOARCH=arm64 CGO_ENABLED=0 go build $(LDFLAGS) -o dist/wt-darwin-arm64    ./cmd/wt/
	GOOS=darwin  GOARCH=amd64 CGO_ENABLED=0 go build $(LDFLAGS) -o dist/wt-darwin-amd64    ./cmd/wt/
	GOOS=linux   GOARCH=amd64 CGO_ENABLED=0 go build $(LDFLAGS) -o dist/wt-linux-amd64     ./cmd/wt/
	GOOS=linux   GOARCH=arm64 CGO_ENABLED=0 go build $(LDFLAGS) -o dist/wt-linux-arm64     ./cmd/wt/
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build $(LDFLAGS) -o dist/wt-windows-amd64.exe ./cmd/wt/
	@cd dist && sha256sum wt-* > checksums.txt 2>/dev/null || shasum -a 256 wt-* > checksums.txt
	@echo "Built all binaries in dist/"
	@echo "Checksums:"
	@cat dist/checksums.txt
