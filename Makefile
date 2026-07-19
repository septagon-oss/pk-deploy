SHELL := /bin/bash
.SHELLFLAGS := -ec
GOWORK_FILE := $(firstword $(wildcard ../go.work))
ifeq ($(GOWORK_FILE),)
GO_WORK := GOWORK=off
else
GO_WORK := GOWORK=$(abspath $(GOWORK_FILE))
endif
GO_ENV ?= $(GO_WORK) GOTMPDIR=$(CURDIR)/.tmp-go-tmp TMPDIR=$(CURDIR)/.tmp-go-tmp
STATICCHECK_VERSION ?= v0.7.0
STATICCHECK ?= go run honnef.co/go/tools/cmd/staticcheck@$(STATICCHECK_VERSION)
TMPDIRS := .tmp-go-tmp

.PHONY: test vet staticcheck fitness verify example

$(TMPDIRS):
	@mkdir -p $@

test: | $(TMPDIRS)
	$(GO_ENV) go test ./...

vet: | $(TMPDIRS)
	$(GO_ENV) go vet ./...

staticcheck: | $(TMPDIRS)
	$(GO_ENV) GOFLAGS=-buildvcs=false $(STATICCHECK) ./...

fitness: | $(TMPDIRS)
	$(GO_ENV) go test -race -count=1 ./pkg/architecture ./pkg/deploy ./pkg/job ./pkg/evidence ./pkg/metrics ./pkg/worker

verify: test vet staticcheck fitness

example: | $(TMPDIRS)
	$(GO_ENV) go run ./examples/minimal
