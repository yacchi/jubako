.PHONY: build test lint clean fmt vet examples

# Submodules with go.mod (excluding examples which have no tests)
SUBMODULES := . ./format/yaml ./format/toml ./format/jsonc

# Build
build:
	go build ./...

# Run examples (verify they compile and execute)
examples:
	@echo "Running examples..."
	@for dir in examples/*/; do \
		echo "  Running $$dir..."; \
		go run "./$$dir" || exit 1; \
	done
	@echo "All examples passed."

# Test all modules
test:
	@for mod in $(SUBMODULES); do \
		echo "Testing $$mod..."; \
		(cd $$mod && go test -v -race ./...) || exit 1; \
	done

# Test with coverage (all modules combined)
test-cover:
	@echo "mode: atomic" > coverage.out
	@for mod in $(SUBMODULES); do \
		echo "Testing $$mod with coverage..."; \
		(cd $$mod && go test -v -race -coverprofile=coverage.tmp -covermode=atomic ./...) || exit 1; \
		if [ -f "$$mod/coverage.tmp" ]; then \
			tail -n +2 "$$mod/coverage.tmp" >> coverage.out; \
			rm -f "$$mod/coverage.tmp"; \
		fi; \
	done
	go tool cover -html=coverage.out -o coverage.html

# Lint
lint: vet
	@which golangci-lint > /dev/null || (echo "golangci-lint not found" && exit 1)
	golangci-lint run

# Vet
vet:
	go vet ./...

# Format
fmt:
	go fmt ./...

# Clean
clean:
	rm -f coverage.out coverage.html

# Tidy dependencies
tidy:
	go mod tidy
