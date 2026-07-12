# The local gate that mirrors CI exactly. If `make check` passes, CI passes —
# the two must not be able to differ, or they eventually will (I tripped CI
# twice by running a subset locally: golangci once, gofmt once). Every step here
# is the same check .github/workflows/ci.yml and lint.yml run, in the same order.
#
# Run `make check` before every push. `make hooks` installs it as a pre-push
# hook so it isn't a thing to remember.

GOLANGCI_VERSION := v2.12.2
COVERAGE_FLOOR := 75.0

.PHONY: check gofmt vet test coverage lint hooks

check: gofmt vet test coverage lint
	@echo "OK: local gate passed (mirrors CI)"

# gofmt -l lists files that are not gofmt-clean; any output is a failure.
gofmt:
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "FAIL gofmt: these files are not gofmt-clean:"; echo "$$unformatted"; exit 1; \
	fi
	@echo "ok gofmt"

vet:
	@go vet ./...
	@echo "ok vet"

test:
	@go test -race -coverprofile=coverage.out ./...

# Library-package coverage floor (examples excluded as fixtures), same as CI.
coverage:
	@go test -coverprofile=cover.lib.out $$(go list ./... | grep -v '/examples/') >/dev/null
	@total=$$(go tool cover -func=cover.lib.out | awk '/^total:/ {print substr($$3, 1, length($$3)-1)}'); \
	echo "library coverage: $${total}%"; \
	awk -v t="$$total" 'BEGIN { if (t+0 < $(COVERAGE_FLOOR)) { print "FAIL: library coverage " t "% is below the $(COVERAGE_FLOOR)% floor"; exit 1 } }'

# Same linter and version as .github/workflows/lint.yml. Requires golangci-lint
# installed locally; `make lint` no-ops with a warning if it's absent rather
# than silently skipping (a skipped check is how the gate drifts).
lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; echo "ok lint"; \
	else \
		echo "WARNING: golangci-lint not installed — CI still runs it (pin $(GOLANGCI_VERSION)). Install: https://golangci-lint.run/welcome/install/"; \
		exit 1; \
	fi

# Install `make check` as a git pre-push hook, so the gate runs without having
# to remember it.
hooks:
	@printf '#!/bin/sh\nexec make check\n' > .git/hooks/pre-push
	@chmod +x .git/hooks/pre-push
	@echo "installed .git/hooks/pre-push -> make check"
