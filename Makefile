SHELL := /bin/sh

GOFILES := $(shell find . -name '*.go' -not -path './vendor/*')
TOOLBIN := $(shell go env GOPATH)/bin

.PHONY: help fmt fmt-check vet test lint staticcheck gosec vuln tools ci

help:
	@printf '%s\n' \
		"Targets:" \
		"  fmt          Format Go files with gofmt" \
		"  fmt-check    Verify gofmt produces no changes" \
		"  vet          Run go vet" \
		"  test         Run go test" \
		"  lint         Run golangci-lint if available" \
		"  staticcheck  Run staticcheck if available" \
		"  gosec        Run gosec if available" \
		"  vuln         Run govulncheck if available" \
		"  tools        Install golangci-lint, staticcheck, gosec, govulncheck" \
		"  ci           Run fmt-check, vet, test, lint, staticcheck, gosec, vuln"

fmt:
	@if [ -n "$(GOFILES)" ]; then gofmt -w $(GOFILES); fi

fmt-check:
	@if [ -n "$(GOFILES)" ]; then \
		if [ -n "$$(gofmt -l $(GOFILES))" ]; then \
			echo "gofmt needed:"; \
			gofmt -l $(GOFILES); \
			exit 1; \
		fi; \
	fi

vet:
	go vet ./...

test:
	go test ./...

lint:
	@if [ -x "$(TOOLBIN)/golangci-lint" ]; then \
		"$(TOOLBIN)/golangci-lint" run ./...; \
	else \
		echo "golangci-lint not found; skipping"; \
	fi

staticcheck:
	@if [ -x "$(TOOLBIN)/staticcheck" ]; then \
		"$(TOOLBIN)/staticcheck" ./...; \
	else \
		echo "staticcheck not found; skipping"; \
	fi

gosec:
	@if [ -x "$(TOOLBIN)/gosec" ]; then \
		"$(TOOLBIN)/gosec" ./...; \
	else \
		echo "gosec not found; skipping"; \
	fi

vuln:
	@if [ -x "$(TOOLBIN)/govulncheck" ]; then \
		"$(TOOLBIN)/govulncheck" ./...; \
	else \
		echo "govulncheck not found; skipping"; \
	fi

tools:
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.64.8
	go install honnef.co/go/tools/cmd/staticcheck@v0.6.1
	go install github.com/securego/gosec/v2/cmd/gosec@v2.22.11
	go install golang.org/x/vuln/cmd/govulncheck@v1.1.4

ci: fmt-check vet test lint staticcheck gosec vuln
