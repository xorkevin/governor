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

COVERAGE_ARGS=-cover -covermode atomic -coverprofile $(COVERAGE_OUT)

.PHONY: test testcover coverage cover

test: ## Run tests
	go test -trimpath -ldflags "-w -s" -race $(TEST_ARGS) $(TEST_PACKAGE)

testcover: ## Run tests with coverage
	go test -trimpath -ldflags "-w -s" -race $(COVERAGE_ARGS) $(TEST_ARGS) $(TEST_PACKAGE)

coverage: ## Create coverage report
	go tool cover -html $(COVERAGE_OUT) -o $(COVERAGE_HTML)

cover: testcover coverage ## Test with coverage

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
