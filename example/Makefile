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
BIN_DIR=bin
BIN_PATH=$(BIN_DIR)/$(BIN_NAME)

.PHONY: clean build

clean:
	if [ -d $(BIN_DIR) ]; then rm -r $(BIN_DIR); fi

build:
	mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 go build -trimpath -ldflags "-w -s -X main.GitHash=$$(git rev-parse --verify HEAD)" -o $(BIN_PATH) $(MAIN_PATH)

# Docker
DOCKER_NAME=governor
DOCKER_VERSION=0.2.7
DOCKER_IMAGE?=$(DOCKER_NAME):$(DOCKER_VERSION)
DOCKER_FILE=cmd/Dockerfile

.PHONY: build-docker publish-docker docker

build-docker:
	docker build -f $(DOCKER_FILE) -t $(DOCKER_IMAGE) -t $(DOCKER_NAME):latest .

publish-docker:
	docker push $(DOCKER_IMAGE)

docker: build-docker publish-docker
