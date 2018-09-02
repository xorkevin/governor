# METADATA
VERSION=v0.1.0

# CMD
BIN_DIR=bin
BIN_NAME=gov
MAIN_PATH=cmd/gov/main.go
BIN_PATH=$(BIN_DIR)/$(BIN_NAME)

# DOCKER
IMAGE_NAME=governor

.PHONY: all test fmt vet prepare dev clean build-bin build build-docker produp proddown devup devdown docker-clean

all: build

test:
	go test -cover ./...

fmt:
	go fmt ./...

vet:
	go vet ./...

prepare: fmt vet

dev:
	go run -ldflags "-X main.GitHash=$$(git rev-parse --verify HEAD)" $(MAIN_PATH) --config configdev

clean:
	if [ -d $(BIN_DIR) ]; then rm -r $(BIN_DIR); fi

build-bin:
	mkdir -p $(BIN_DIR)
	if [ -f $(BIN_PATH) ]; then rm $(BIN_PATH); fi
	CGO_ENABLED=0 go build -a -tags netgo -ldflags "-w -s -X main.GitHash=$$(git rev-parse --verify HEAD)" -o $(BIN_PATH) $(MAIN_PATH)

build: clean build-bin

build-docker:
	docker build -f ./cmd/gov/Dockerfile -t $(IMAGE_NAME):$(VERSION) -t $(IMAGE_NAME):latest .


## docker
produp:
	docker-compose -f docker-compose.yaml -f docker-compose-app.yaml up -d

proddown:
	docker-compose -f docker-compose.yaml -f docker-compose-app.yaml down

devup:
	docker-compose -f docker-compose.yaml -f docker-compose-dev.yaml up -d

devdown:
	docker-compose -f docker-compose.yaml -f docker-compose-dev.yaml down

docker-clean:
	if [ "$$(docker ps -q -f status=running)" ]; \
		then docker stop $$(docker ps -q -f status=running); fi
	if [ "$$(docker ps -q -f status=restarting)" ]; \
		then docker stop $$(docker ps -q -f status=restarting); fi
	if [ "$$(docker ps -q -f status=exited)" ]; \
		then docker rm $$(docker ps -q -f status=exited); fi
	if [ "$$(docker ps -q -f status=created)" ]; \
		then docker rm $$(docker ps -q -f status=created); fi
