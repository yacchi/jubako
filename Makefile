.PHONY: build test lint clean fmt vet

# Build
build:
	go build ./...

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
