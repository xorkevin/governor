.PHONY: all

all: build

.PHONY: devinit devinit2 devconfig devup devdown

devinit:
	./plan.sh
	./register.sh
	./connect.sh

devup:
	kubectl apply -k base

devdown:
	kubectl delete -k base

devinit2:
	./register2.sh

devconfig:
	./setsmtpauth.sh

# CMD
BIN_NAME=governor
MAIN_PATH=cmd/main.go
BIN_DIR=./bin
BIN_PATH=$(BIN_DIR)/$(BIN_NAME)

.PHONY: clean build

clean:
	if [ -d $(BIN_DIR) ]; then rm -r $(BIN_DIR); fi

build:
	mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 go build -trimpath -ldflags "-w -s -X main.GitHash=$$(git rev-parse --verify HEAD)" -o $(BIN_PATH) $(MAIN_PATH)

# Docker
DOCKER_NAME=docker.pkg.dev.home:8080/governor

DOCKER_MAJOR_VERSION=0.2
DOCKER_VERSION=0.2.7-0

DOCKER_LATEST_IMAGE=$(DOCKER_NAME):latest
DOCKER_MAJOR_IMAGE=$(DOCKER_NAME):$(DOCKER_MAJOR_VERSION)
DOCKER_IMAGE=$(DOCKER_NAME):$(DOCKER_VERSION)

.PHONY: build-docker publish-docker docker

build-docker:
	docker build -t $(DOCKER_IMAGE) -t $(DOCKER_MAJOR_IMAGE) -t $(DOCKER_LATEST_IMAGE) .

publish-docker:
	docker push $(DOCKER_IMAGE)
	docker push $(DOCKER_MAJOR_IMAGE)
	docker push $(DOCKER_LATEST_IMAGE)

docker: build-docker publish-docker

.PHONY: skaffoldinit

skaffoldinit:
	skaffold config set -g update-check false

.PHONY: dev setup tail

dev: build
	skaffold run

setup: build
	$(BIN_PATH) setup

tail:
	kubectl -n governor governor
