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

.PHONY: test testunit testcover testcoverunit coverage cover coverunit

test: ## Run all tests
	./test.sh $(TEST_ARGS) $(TEST_PACKAGE)

testunit: ## Runs unit tests
	go test -trimpath -ldflags "-w -s" -race -short $(TEST_ARGS) $(TEST_PACKAGE)

testcover: ## Run all tests with coverage
	./test.sh $(COVERAGE_ARGS) $(TEST_ARGS) $(TEST_PACKAGE)

testcoverunit: ## Run unit tests with coverage
	go test -trimpath -ldflags "-w -s" -race -short $(COVERAGE_ARGS) $(TEST_ARGS) $(TEST_PACKAGE)

coverage: ## Create coverage report
	go tool cover -html $(COVERAGE_OUT) -o $(COVERAGE_HTML)

cover: testcover coverage ## Test with coverage

coverunit: testcoverunit coverage ## Test with coverage

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
