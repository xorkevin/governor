## PROLOG

.PHONY: help all

CMDNAME=governor
CMDDESC=go service framework

help: ## Print this help
	@./help.sh '$(CMDNAME)' '$(CMDDESC)'

all: test ## Default

## TESTS

TEST_ARGS?=
TEST_PACKAGE?=./...

COVERAGE_OUT?=cover.out
COVERAGE_HTML?=coverage.html

ifneq ($(TEST_RACE),)
	TEST_ARGS+=-race
endif

ifneq ($(TEST_COVER),)
	TEST_ARGS+=-cover -covermode atomic -coverprofile $(COVERAGE_OUT)
endif

.PHONY: test coverage cover

test: ## Run tests
	go test -trimpath -ldflags "-w -s" $(TEST_ARGS) $(TEST_PACKAGE)

coverage: ## View test coverage
	go tool cover -html $(COVERAGE_OUT) -o $(COVERAGE_HTML)

cover: test coverage ## Create coverage report

## FMT

.PHONY: fmt vet prepare

fmt: ## Format code
	goimports -w .

vet: ## Lint code
	go vet ./...

prepare: fmt vet ## Prepare code for PR

## CODEGEN

GENSRC=$(shell find . -name '*_gen.go')

.PHONY: generate gen cleangen

generate: ## Run go generate
	go generate ./...

gen: generate fmt ## Run codegen

cleangen: ## Remove generated code
	rm $(GENSRC)
