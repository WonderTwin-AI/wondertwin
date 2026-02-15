.PHONY: build build-twins build-all clean test vet goreleaser-check release-local

VERSION ?= dev
GORELEASER ?= goreleaser
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

goreleaser-check: ## Validate Goreleaser config
	$(GORELEASER) check

release-local: ## Build cross-platform release artifacts into dist/ using Goreleaser
	$(GORELEASER) release --snapshot --clean
