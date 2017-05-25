# METADATA
VERSION=v0.1.0
MODE=INFO
API_PORT=8080
FSS_PORT=3000
BASEDIR=public


# CMD
BIN_OUT=bin

## auth
AUTH_NAME=auth
AUTH_PATH=cmd/auth/main.go
AUTH_BIN_PATH=$(BIN_OUT)/$(AUTH_NAME)

## fsserve
FSSERVE_NAME=fsserve
FSSERVE_PATH=cmd/fsserve/main.go
FSSERVE_BIN_PATH=$(BIN_OUT)/$(FSSERVE_NAME)

# DEV
POSTGRES_URL_DEV="user=postgres password=admin dbname=governor host=localhost port=5432 sslmode=disable"

# DOCKER
DOCKER_NETWORK=governornetwork
AUTH_IMAGE_NAME=governorauth
AUTH_CONTAINER_NAME=gauth
FSSERVE_IMAGE_NAME=governorfsserver
FSSERVE_CONTAINER_NAME=gfs
POSTGRES_URL_DOCKER="user=postgres password=admin dbname=governor host=$(POSTGRES_CONTAINER) port=5432 sslmode=disable"


# DEV_POSTGRES
POSTGRES_VOLUME=governorpgvol
POSTGRES_CONTAINER=gpostgres
POSTGRES_PASS=admin



all: build


test:
	go test -cover $$(glide novendor)

dev:
	VERSION=$(VERSION) MODE=DEBUG POSTGRES_URL=$(POSTGRES_URL_DEV) go run $(AUTH_PATH)

dev-fsserve:
	BASEDIR=$(BASEDIR) go run $(FSSERVE_PATH)

clean:
	if [ -d $(BIN_OUT) ]; then rm -r $(BIN_OUT); fi

build-auth:
	mkdir -p $(BIN_OUT)
	if [ -f $(AUTH_BIN_PATH) ]; then rm $(AUTH_BIN_PATH);	fi
	CGO_ENABLED=0 go build -a -tags netgo -ldflags '-w -s' -o $(AUTH_BIN_PATH) $(AUTH_PATH)

build-fsserve:
	mkdir -p $(BIN_OUT)
	if [ -f $(FSSERVE_BIN_PATH) ]; then rm $(FSSERVE_BIN_PATH); fi
	CGO_ENABLED=0 go build -a -tags netgo -ldflags '-w -s' -o $(FSSERVE_BIN_PATH) $(FSSERVE_PATH)

build: clean build-auth build-fsserve


## docker

docker-setup:
	docker network create -d bridge $(DOCKER_NETWORK)

docker-build: build
	docker build -f Dockerfile.auth -t $(AUTH_IMAGE_NAME):$(VERSION) .
	docker build -f Dockerfile.auth -t $(AUTH_IMAGE_NAME) .
	docker build -f Dockerfile.fs -t $(FSSERVE_IMAGE_NAME) .

docker-run:
	docker run -d --name $(AUTH_CONTAINER_NAME) -e VERSION=$(VERSION) -e MODE=$(MODE) -e POSTGRES_URL=$(POSTGRES_URL_DOCKER) --network=$(DOCKER_NETWORK) -p $(API_PORT):$(API_PORT) $(AUTH_IMAGE_NAME)
	docker run -d --name $(FSSERVE_CONTAINER_NAME) -e BASEDIR=$(BASEDIR) -v $$(pwd)/$(BASEDIR):/$(BASEDIR) --network=$(DOCKER_NETWORK) -p $(FSS_PORT):$(FSS_PORT) $(FSSERVE_IMAGE_NAME)

docker-stop:
	if [ "$$(docker ps -q -f name=$(AUTH_CONTAINER_NAME) -f status=running)" ]; then docker stop $(AUTH_CONTAINER_NAME); fi
	if [ "$$(docker ps -q -f name=$(FSSERVE_CONTAINER_NAME) -f status=running)" ]; then docker stop $(FSSERVE_CONTAINER_NAME); fi
	if [ "$$(docker ps -q -f name=$(AUTH_CONTAINER_NAME) -f status=exited)" ]; then docker rm $(AUTH_CONTAINER_NAME); fi
	if [ "$$(docker ps -q -f name=$(FSSERVE_CONTAINER_NAME) -f status=exited)" ]; then docker rm $(FSSERVE_CONTAINER_NAME); fi

docker-restart: docker-stop docker-run

docker: docker-build docker-restart


## postgres
pg-setup:
	docker volume create --name $(POSTGRES_VOLUME)

pg-run:
	docker run -d --name $(POSTGRES_CONTAINER) --network=$(DOCKER_NETWORK) -p 5432:5432 -v $(POSTGRES_VOLUME):/var/lib/postgresql/data -e POSTGRES_PASSWORD=$(POSTGRES_PASS) postgres:alpine

pg-stop:
	if [ "$$(docker ps -q -f name=$(POSTGRES_CONTAINER) -f status=running)" ]; then docker stop $(POSTGRES_CONTAINER); fi
	if [ "$$(docker ps -q -f name=$(POSTGRES_CONTAINER) -f status=exited)" ]; then docker rm $(POSTGRES_CONTAINER); fi

pg-restart: pg-stop pg-run
