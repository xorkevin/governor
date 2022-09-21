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
COVERAGE?=cover.out

.PHONY: test coverage cover bench

test: ## Run tests
	go test -race -trimpath -ldflags "-w -s" -cover -covermode atomic -coverprofile $(COVERAGE) $(TEST_ARGS) $(TEST_PACKAGE)

coverage: ## View test coverage
	go tool cover -html $(COVERAGE)

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
