TEST_ARGS=
COVERAGE=cover.out
COVERAGE_ARGS=-covermode count -coverprofile $(COVERAGE)
BENCHMARK_ARGS=-benchtime 5s -benchmem

.PHONY: test coverage cover bench

test:
	go test $(TEST_ARGS) -cover $(COVERAGE_ARGS) ./...

coverage:
	go tool cover -html $(COVERAGE)

cover: test coverage

bench:
	go test -bench . $(BENCHMARK_ARGS)

.PHONY: fmt vet prepare

fmt:
	go fmt ./...

vet:
	go vet ./...

prepare: fmt vet

GENSRC=$(shell find . -name '*_gen.go')

.PHONY: generate gen cleangen

generate:
	go generate ./...

gen: generate fmt

cleangen:
	rm $(GENSRC)
