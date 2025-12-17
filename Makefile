.PHONY: build test lint clean fmt vet examples setup prepare check-release check-ci tidy ci-test ci-lint

GOCACHE ?= $(CURDIR)/.gocache
export GOCACHE

# Find all modules with go.mod (excluding .gopath and vendor)
ALL_MODULES := $(shell find . -name "go.mod" -not -path "./.gopath/*" -not -path "./vendor/*" -exec dirname {} \; | sort)

# Submodules with tests (modules containing *_test.go files)
SUBMODULES := $(shell for mod in $(ALL_MODULES); do \
	if find "$$mod" -maxdepth 1 -name "*_test.go" -print -quit 2>/dev/null | grep -q .; then \
		echo "$$mod"; \
	elif find "$$mod" -mindepth 1 -name "*_test.go" -print -quit 2>/dev/null | grep -q .; then \
		echo "$$mod"; \
	fi; \
done)

# Dependent modules (modules that depend on root, i.e., ALL_MODULES excluding root ".")
DEP_MODULES := $(filter-out .,$(ALL_MODULES))

# Setup go.work for local development (required before build/test)
# Note: Use 'make prepare' for idempotent setup (CI and local)
setup:
	@if [ ! -f go.work ]; then \
		echo "Creating go.work..."; \
		go work init; \
		for mod in $(ALL_MODULES); do \
			go work use $$mod; \
		done; \
		echo "go.work created."; \
	else \
		echo "go.work already exists."; \
	fi

# Prepare development environment (idempotent, safe to run multiple times)
# - Creates/updates go.work with all modules and replace directives
# - Replace directives redirect versioned dependencies to local paths
# Use this in CI and before local development
prepare:
	@echo "Preparing development environment..."
	@echo "go 1.24" > go.work
	@echo "" >> go.work
	@echo "use (" >> go.work
	@for mod in $(ALL_MODULES); do \
		echo "	$$mod" >> go.work; \
	done
	@echo ")" >> go.work
	@echo "" >> go.work
	@echo "replace (" >> go.work
	@for mod in $(ALL_MODULES); do \
		modpath=$$(head -1 "$$mod/go.mod" | sed 's/^module //'); \
		echo "	$$modpath v0.1.0 => $$mod" >> go.work; \
	done
	@echo ")" >> go.work
	@echo "go.work created:"
	@cat go.work

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
		(cd $$mod && go test -race -coverprofile=coverage.tmp -covermode=atomic ./...) || exit 1; \
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

# Tidy dependencies for all modules
tidy:
	@for mod in $(ALL_MODULES); do \
		echo "Tidying $$mod..."; \
		(cd $$mod && go mod tidy) || exit 1; \
	done

# Check for release readiness
# - No replace directives in any go.mod files
# - No v0.0.0 dependencies in submodules (placeholder versions)
check-release:
	@echo "Checking release readiness..."
	@errors=0; \
	for mod in $(ALL_MODULES); do \
		if grep -q "^replace " "$$mod/go.mod" 2>/dev/null; then \
			echo "ERROR: $$mod/go.mod contains replace directive"; \
			errors=1; \
		fi; \
	done; \
	for mod in $(DEP_MODULES); do \
		if grep -q "github.com/yacchi/jubako v0\.0\.0" "$$mod/go.mod" 2>/dev/null; then \
			echo "ERROR: $$mod/go.mod references jubako v0.0.0 (update to released version)"; \
			errors=1; \
		fi; \
	done; \
	if [ $$errors -eq 0 ]; then \
		echo "All checks passed. Ready for release."; \
	else \
		echo "Release checks failed."; \
		exit 1; \
	fi

# Check CI readiness (no replace directives, go.work will be generated)
check-ci:
	@echo "Checking CI readiness..."
	@errors=0; \
	for mod in $(ALL_MODULES); do \
		if grep -q "^replace " "$$mod/go.mod" 2>/dev/null; then \
			echo "ERROR: $$mod/go.mod contains replace directive"; \
			errors=1; \
		fi; \
	done; \
	if [ $$errors -eq 0 ]; then \
		echo "All checks passed. CI ready."; \
	else \
		echo "CI checks failed. Remove replace directives and use go.work for local development."; \
		exit 1; \
	fi

# CI test task - run tests with coverage for all modules
ci-test:
	@echo "mode: atomic" > coverage.txt
	@for mod in $(SUBMODULES); do \
		echo "Testing $$mod with coverage..."; \
		(cd $$mod && go test -race -coverprofile=coverage.tmp -covermode=atomic ./...) || exit 1; \
		if [ -f "$$mod/coverage.tmp" ]; then \
			tail -n +2 "$$mod/coverage.tmp" >> coverage.txt; \
			rm -f "$$mod/coverage.tmp"; \
		fi; \
	done
	@echo "Coverage report: coverage.txt"

# CI lint task - run go vet for all modules
ci-lint:
	@for mod in $(SUBMODULES); do \
		echo "Vetting $$mod..."; \
		(cd $$mod && go vet ./...) || exit 1; \
	done
