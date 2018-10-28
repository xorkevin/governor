# METADATA
VERSION=v0.1.0

# CMD
BIN_DIR=bin
BIN_NAME=gov
MAIN_PATH=cmd/gov/main.go
BIN_PATH=$(BIN_DIR)/$(BIN_NAME)

# DOCKER
IMAGE_NAME=governor

EXT?=0

GO=go
ifeq ($(EXT),1)
	GOROOT=$(TOOLCHAIN_GOROOT)
	GO=$(TOOLCHAIN_GOBIN)
endif

.PHONY: all version test fmt vet prepare

all: build

version:
	@$(GO) version
	@$(GO) env

test:
	$(GO) test -cover ./...

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

prepare: fmt vet

.PHONY: dev clean build-bin build

dev:
	$(GO) run -ldflags "-X main.GitHash=$$(git rev-parse --verify HEAD)" $(MAIN_PATH) --config configdev

clean:
	if [ -d $(BIN_DIR) ]; then rm -r $(BIN_DIR); fi

build-bin:
	mkdir -p $(BIN_DIR)
	if [ -f $(BIN_PATH) ]; then rm $(BIN_PATH); fi
	CGO_ENABLED=0 $(GO) build -a -tags netgo -ldflags "-w -s -X main.GitHash=$$(git rev-parse --verify HEAD)" -o $(BIN_PATH) $(MAIN_PATH)

build: clean build-bin

## gen
.PHONY: gen

gen:
	./servicedef-gen.sh

## docker
.PHONY: build-docker produp proddown devup devdown docker-clean

build-docker:
	docker build -f ./cmd/gov/Dockerfile -t $(IMAGE_NAME):$(VERSION) -t $(IMAGE_NAME):latest .

produp:
	docker-compose -f dc.main.yaml -f dc.prod.yaml -f dc.compose.yaml up -d

proddown:
	docker-compose -f dc.main.yaml -f dc.prod.yaml -f dc.compose.yaml down

devup:
	docker-compose -f dc.main.yaml -f dc.dev.yaml up -d

devdown:
	docker-compose -f dc.main.yaml -f dc.dev.yaml down

docker-clean:
	if [ "$$(docker ps -q -f status=running)" ]; \
		then docker stop $$(docker ps -q -f status=running); fi
	if [ "$$(docker ps -q -f status=restarting)" ]; \
		then docker stop $$(docker ps -q -f status=restarting); fi
	if [ "$$(docker ps -q -f status=exited)" ]; \
		then docker rm $$(docker ps -q -f status=exited); fi
	if [ "$$(docker ps -q -f status=created)" ]; \
		then docker rm $$(docker ps -q -f status=created); fi

## service
SERVICE_STACK=governor
.PHONY: launch danger-land
launch:
	docker stack deploy -c defs/dc.gov.yaml $(SERVICE_STACK)

danger-land:
	docker stack rm $(SERVICE_STACK)

## local go installation
TOOLCHAIN_DIR=toolchain
TOOLCHAIN_GO_DIR=$(TOOLCHAIN_DIR)/go
TOOLCHAIN_TAR=$(TOOLCHAIN_DIR)/go.tar.gz
TOOLCHAIN_GOROOT=$(TOOLCHAIN_GO_DIR)/go
TOOLCHAIN_GOBIN=$(TOOLCHAIN_GOROOT)/bin/go

TOOLCHAIN_URL=https://dl.google.com/go/go1.11.linux-amd64.tar.gz

.PHONY: toolchain toolclean

toolchain:
	mkdir -p $(TOOLCHAIN_DIR)
	if [ ! -x $(TOOLCHAIN_GO_DIR) ]; then \
		wget -q --show-progress $(TOOLCHAIN_URL) -O $(TOOLCHAIN_TAR); \
		mkdir -p $(TOOLCHAIN_GO_DIR); \
		tar xzf $(TOOLCHAIN_TAR) -C $(TOOLCHAIN_GO_DIR); \
	fi;

toolclean:
	rm -rf $(TOOLCHAIN_DIR)
