.PHONY: build clean test vet

build: ## Build the wt CLI binary
	go build -o bin/wt ./cmd/wt/
	@echo "Built bin/wt"

clean: ## Remove build artifacts
	rm -rf bin/

test: ## Run all tests
	go test ./... -count=1

vet: ## Run go vet
	go vet ./...
