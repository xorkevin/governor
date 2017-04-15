# METADATA
VERSION=v0.1.0
MODE=INFO
API_PORT=8080
FSS_PORT=3000
BASEDIR=public
POSTGRES_URL="user=postgres password=admin dbname=governor host=localhost port=5432 sslmode=disable"


# CMD
BIN_OUT=bin

## serve
SERVE_NAME=serve
SERVE_PATH=cmd/serve/main.go
SERVE_BIN_PATH=$(BIN_OUT)/$(SERVE_NAME)

## fsserve
FSSERVE_NAME=fsserve
FSSERVE_PATH=cmd/fsserve/main.go
FSSERVE_BIN_PATH=$(BIN_OUT)/$(FSSERVE_NAME)


# DOCKER
SERVE_IMAGE_NAME=governorserver
SERVE_CONTAINER_NAME=sgovernor
FSSERVE_IMAGE_NAME=governorfsserver
FSSERVE_CONTAINER_NAME=fssgovernor


# DEV_POSTGRES
POSTGRES_VOLUME=governorpgvol
POSTGRES_CONTAINER=governorpg
POSTGRES_PASS=admin



all: build


test:
	go test -cover $$(glide novendor)

dev:
	VERSION=$(VERSION) MODE=DEBUG POSTGRES_URL=$(POSTGRES_URL) go run $(SERVE_PATH)

dev-fsserve:
	BASEDIR=$(BASEDIR) go run $(FSSERVE_PATH)

clean:
	if [ -d $(BIN_OUT) ]; then rm -r $(BIN_OUT); fi

build-serve:
	mkdir -p $(BIN_OUT)
	if [ -f $(SERVE_BIN_PATH) ]; then rm $(SERVE_BIN_PATH);	fi
	CGO_ENABLED=0 go build -a -tags netgo -ldflags '-w -s' -o $(SERVE_BIN_PATH) $(SERVE_PATH)

build-fsserve:
	mkdir -p $(BIN_OUT)
	if [ -f $(FSSERVE_BIN_PATH) ]; then rm $(FSSERVE_BIN_PATH); fi
	CGO_ENABLED=0 go build -a -tags netgo -ldflags '-w -s' -o $(FSSERVE_BIN_PATH) $(FSSERVE_PATH)

build: clean build-serve build-fsserve


## docker
docker-build: build
	docker build -t $(SERVE_IMAGE_NAME):$(VERSION) .
	docker build -t $(SERVE_IMAGE_NAME) .
	docker build -f Dockerfile.fs -t $(FSSERVE_IMAGE_NAME) .

docker-run:
	docker run -d --name $(SERVE_CONTAINER_NAME) -e VERSION=$(VERSION) -e MODE=$(MODE) -e POSTGRES_URL=$(POSTGRES_URL) -p $(API_PORT):$(API_PORT) $(SERVE_IMAGE_NAME)
	docker run -d --name $(FSSERVE_CONTAINER_NAME) -e BASEDIR=$(BASEDIR) -v $$(pwd)/$(BASEDIR):/$(BASEDIR) -p $(FSS_PORT):$(FSS_PORT) $(FSSERVE_IMAGE_NAME)

docker-stop:
	if [ "$$(docker ps -q -f name=$(SERVE_CONTAINER_NAME) -f status=running)" ]; then docker stop $(SERVE_CONTAINER_NAME); fi
	if [ "$$(docker ps -q -f name=$(FSSERVE_CONTAINER_NAME) -f status=running)" ]; then docker stop $(FSSERVE_CONTAINER_NAME); fi
	if [ "$$(docker ps -q -f name=$(SERVE_CONTAINER_NAME) -f status=exited)" ]; then docker rm $(SERVE_CONTAINER_NAME); fi
	if [ "$$(docker ps -q -f name=$(FSSERVE_CONTAINER_NAME) -f status=exited)" ]; then docker rm $(FSSERVE_CONTAINER_NAME); fi

docker-restart: docker-stop docker-run

docker: docker-build docker-restart


## postgres
pg-setup:
	docker volume create --name $(POSTGRES_VOLUME)

pg-run:
	docker run -d --name $(POSTGRES_CONTAINER) -p 5432:5432 -v $(POSTGRES_VOLUME):/var/lib/postgresql/data -e POSTGRES_PASSWORD=$(POSTGRES_PASS) postgres:alpine

pg-stop:
	if [ "$$(docker ps -q -f name=$(POSTGRES_CONTAINER) -f status=running)" ]; then docker stop $(POSTGRES_CONTAINER); fi
	if [ "$$(docker ps -q -f name=$(POSTGRES_CONTAINER) -f status=exited)" ]; then docker rm $(POSTGRES_CONTAINER); fi

pg-restart: pg-stop pg-run
