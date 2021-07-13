## PROLOG

.PHONY: help all

CMDNAME=governor
CMDDESC=go service framework

help: ## Print this help
	@./help.sh '$(CMDNAME)' '$(CMDDESC)'

all: test ## Default

## TESTS

TEST_ARGS=
COVERAGE=cover.out
COVERAGE_ARGS=-covermode count -coverprofile $(COVERAGE)
BENCHMARK_ARGS=-benchtime 5s -benchmem

.PHONY: test coverage cover bench

test: ## Run tests
	go test $(TEST_ARGS) -cover $(COVERAGE_ARGS) ./...

coverage: ## View test coverage
	go tool cover -html $(COVERAGE)

cover: test coverage ## Create coverage report

bench: ## Run benchmarks
	go test -bench . $(BENCHMARK_ARGS)

## FMT

.PHONY: fmt vet prepare

fmt: ## Run go fmt
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
