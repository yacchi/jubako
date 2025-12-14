.PHONY: build test lint clean fmt vet examples

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

# Test
test:
	go test -v -race ./...

# Test with coverage
test-cover:
	go test -v -race -coverprofile=coverage.out ./...
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
