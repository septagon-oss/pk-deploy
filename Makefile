SHELL := /bin/bash
.SHELLFLAGS := -ec
GOWORK_FILE := $(firstword $(wildcard ../go.work))
ifeq ($(GOWORK_FILE),)
GO_WORK := GOWORK=off
else
GO_WORK := GOWORK=$(abspath $(GOWORK_FILE))
endif
GO_ENV ?= $(GO_WORK) GOCACHE=$(CURDIR)/.tmp-go-cache GOTMPDIR=$(CURDIR)/.tmp-go-tmp
STATICCHECK ?= go run honnef.co/go/tools/cmd/staticcheck@latest
STATICCHECK_CACHE ?= $(CURDIR)/.tmp-staticcheck-cache

.PHONY: test vet staticcheck fitness verify example

.tmp-go-cache .tmp-go-tmp .tmp-staticcheck-cache:
	mkdir -p $@

test: .tmp-go-cache .tmp-go-tmp
	$(GO_ENV) go test ./...

vet: .tmp-go-cache .tmp-go-tmp
	$(GO_ENV) go vet ./...

staticcheck: .tmp-go-cache .tmp-go-tmp .tmp-staticcheck-cache
	XDG_CACHE_HOME=$(STATICCHECK_CACHE) $(GO_ENV) GOFLAGS=-buildvcs=false $(STATICCHECK) ./...

fitness: .tmp-go-cache .tmp-go-tmp
	$(GO_ENV) go test -race -count=1 ./pkg/architecture ./pkg/deploy ./pkg/job ./pkg/evidence ./pkg/metrics ./pkg/worker

verify: test vet staticcheck fitness

example:
	$(GO_ENV) go run ./examples/minimal
