.PHONY: build test lint clean fmt vet examples readme-extract readme-examples check-examples setup prepare check-release check-ci tidy ci-test ci-lint ci-release version update-version verify-version check-tags release-tag release

GOCACHE ?= $(CURDIR)/.gocache
export GOCACHE

# Base module name from go.mod
BASE_MODULE := $(shell head -1 go.mod | sed 's/^module //')

# Version from version.txt (without 'v' prefix)
VERSION_NUM := $(shell cat version.txt 2>/dev/null || echo "0.0.0")
VERSION := v$(VERSION_NUM)

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

# Show current version
version:
	@echo $(VERSION)

# Prepare development environment (idempotent, safe to run multiple times)
# - Creates/updates go.work with all modules and replace directives
# - Replace directives redirect versioned dependencies to local paths
# Use this in CI and before local development
prepare:
	@echo "Preparing development environment (version: $(VERSION))..."
	@echo "go 1.24.0" > go.work
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
		echo "	$$modpath $(VERSION) => $$mod" >> go.work; \
	done
	@echo ")" >> go.work
	@echo "go.work created:"
	@cat go.work

# Update all go.mod files to use VERSION from version.txt
# Dynamically updates all module references based on BASE_MODULE and ALL_MODULES
update-version:
	@echo "Updating all modules to $(VERSION)..."
	@for mod in $(DEP_MODULES); do \
		echo "Updating $$mod/go.mod..."; \
		for target in $(ALL_MODULES); do \
			targetpath=$$(head -1 "$$target/go.mod" | sed 's/^module //'); \
			sed -i '' "s|$$targetpath v[0-9.]*|$$targetpath $(VERSION)|g" "$$mod/go.mod"; \
		done; \
	done
	@echo "All modules updated to $(VERSION)"

# Verify go.mod versions match version.txt (used by CI)
# Checks that all dependent modules reference BASE_MODULE at VERSION
verify-version:
	@echo "Verifying go.mod versions match $(VERSION)..."
	@errors=0; \
	for mod in $(DEP_MODULES); do \
		if [ -f "$$mod/go.mod" ]; then \
			if ! grep -q "$(BASE_MODULE) $(VERSION)" "$$mod/go.mod" 2>/dev/null; then \
				echo "ERROR: $$mod/go.mod does not reference $(BASE_MODULE) $(VERSION)"; \
				errors=1; \
			fi; \
		fi; \
	done; \
	if [ $$errors -ne 0 ]; then \
		echo "Version mismatch. Run 'make update-version' and commit."; \
		exit 1; \
	fi
	@echo "All go.mod files reference $(VERSION)"

# Check if release tags already exist
check-tags:
	@echo "Checking if tags for $(VERSION) exist..."
	@if git rev-parse "$(VERSION)" >/dev/null 2>&1; then \
		echo "Tag $(VERSION) already exists"; \
		exit 1; \
	fi
	@for mod in $(DEP_MODULES); do \
		modname=$$(echo "$$mod" | sed 's|^\./||'); \
		tag="$$modname/$(VERSION)"; \
		if git rev-parse "$$tag" >/dev/null 2>&1; then \
			echo "Tag $$tag already exists"; \
			exit 1; \
		fi; \
	done
	@echo "No existing tags for $(VERSION)"

# Create release tags for all modules (does not push)
release-tag: check-tags
	@echo "Creating release tags for $(VERSION)..."
	@git tag $(VERSION)
	@echo "Created tag: $(VERSION)"
	@for mod in $(DEP_MODULES); do \
		modname=$$(echo "$$mod" | sed 's|^\./||'); \
		tag="$$modname/$(VERSION)"; \
		git tag "$$tag"; \
		echo "Created tag: $$tag"; \
	done
	@echo "All tags created. Push with: git push --tags"

# Full release process (verify + tag + push tags)
release: verify-version release-tag
	@echo "Pushing tags..."
	@git push --tags
	@echo "Release $(VERSION) complete!"

# Build
build:
	go build ./...

# Run examples (verify they compile and execute)
examples:
	@echo "Running examples..."
	@for dir in examples/*/; do \
		if [ "$$(basename $$dir)" != ".readme" ]; then \
			echo "  Running $$dir..."; \
			go run "./$$dir" || exit 1; \
		fi; \
	done
	@echo "All examples passed."

# Extract README code blocks to examples/.readme/
readme-extract:
	@go run ./scripts/extract-readme

# Run extracted README examples
# Creates stub config files that some README examples expect
readme-examples: readme-extract
	@echo "Running README examples..."
	@mkdir -p ~/.config/app
	@touch ~/.config/app/config.yaml
	@touch .app.yaml
	@if [ -d examples/.readme ]; then \
		for dir in examples/.readme/*/; do \
			if [ -d "$$dir" ]; then \
				echo "  Running $$dir..."; \
				go run "./$$dir" || exit 1; \
			fi; \
		done; \
		echo "README examples passed."; \
	else \
		echo "No README examples to run."; \
	fi

# Combined check: existing examples + README examples
check-examples: examples readme-examples
	@echo "All examples verified."

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
# - All go.mod files use VERSION from version.txt
# - No v0.0.0 dependencies
check-release:
	@echo "Checking release readiness (version: $(VERSION))..."
	@errors=0; \
	for mod in $(ALL_MODULES); do \
		if grep -q "^replace " "$$mod/go.mod" 2>/dev/null; then \
			echo "ERROR: $$mod/go.mod contains replace directive"; \
			errors=1; \
		fi; \
	done; \
	for mod in $(DEP_MODULES); do \
		if grep -q "$(BASE_MODULE) v0\.0\.0" "$$mod/go.mod" 2>/dev/null; then \
			echo "ERROR: $$mod/go.mod references $(BASE_MODULE) v0.0.0 (run 'make update-version')"; \
			errors=1; \
		fi; \
		if ! grep -q "$(BASE_MODULE) $(VERSION)" "$$mod/go.mod" 2>/dev/null; then \
			echo "WARNING: $$mod/go.mod may not reference $(VERSION)"; \
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

# CI release task - complete release process for CI
# Skips if tag already exists, otherwise: verify + tag + push
ci-release:
	@echo "CI Release for $(VERSION)..."
	@if git rev-parse "$(VERSION)" >/dev/null 2>&1; then \
		echo "Tag $(VERSION) already exists. Skipping release."; \
		exit 0; \
	fi
	@$(MAKE) verify-version
	@$(MAKE) release-tag
	@echo "Pushing tags..."
	@git push --tags
	@echo "Release $(VERSION) complete!"
