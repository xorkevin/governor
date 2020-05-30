.PHONY: devinit devinit2 devup devdown

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

# CMD
BIN_NAME=governor
MAIN_PATH=cmd/main.go
BIN_DIR=bin
BIN_PATH=$(BIN_DIR)/$(BIN_NAME)

.PHONY: clean build-bin build

clean:
	if [ -d $(BIN_DIR) ]; then rm -r $(BIN_DIR); fi

build-bin:
	mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 go build -trimpath -ldflags "-w -s -X main.GitHash=$$(git rev-parse --verify HEAD)" -o $(BIN_PATH) $(MAIN_PATH)

build: clean build-bin

## docker
DOCKER_IMAGE=governor
DOCKER_VERSION=0.2.7
DOCKER_FILE=./cmd/Dockerfile

.PHONY: build-docker

build-docker:
	docker build -f $(DOCKER_FILE) -t $(DOCKER_IMAGE):$(DOCKER_VERSION) -t $(DOCKER_IMAGE):latest .
