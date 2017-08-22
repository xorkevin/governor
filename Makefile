# METADATA
VERSION=v0.1.0
API_PORT=8080
FSS_PORT=3000
CONFIGDIR=config
PUBLICDIR=public
TEMPLATEDIR=templates
PACKAGE=github.com/hackform/governor


# CMD
BIN_DIR=bin
BIN_NAME=gov
MAIN_PATH=cmd/gov/main.go
BIN_PATH=$(BIN_DIR)/$(BIN_NAME)


# DOCKER
DOCKER_NETWORK=governornetwork
IMAGE_NAME=governor
CONTAINER_NAME=govc


# DEV_POSTGRES
POSTGRES_VOLUME=governorpgvol
POSTGRES_CONTAINER=gpostgres
POSTGRES_PASS=admin

# DEV_REDIS
REDIS_VOLUME=governorredisvol
REDIS_CONTAINER=gredis
REDIS_PASS=admin

# DEV_MINIO
MINIO_VOLUME=governorminiovol
MINIO_VOLUME_CONFIG=governorminioconfigvol
MINIO_CONTAINER=gminio
MINIO_KEY=admin
MINIO_SECRET=adminsecret

# DEV_MAIL
MAIL_CONTAINER=gmail



all: build


test:
	go test -cover $$(go list ./... | grep -v "^$(PACKAGE)/vendor/")

dev:
	go run $(MAIN_PATH) --config configdev

clean:
	if [ -d $(BIN_DIR) ]; then rm -r $(BIN_DIR); fi

build-bin:
	mkdir -p $(BIN_DIR)
	if [ -f $(BIN_PATH) ]; then rm $(BIN_PATH);	fi
	CGO_ENABLED=0 go build -a -tags netgo -ldflags '-w -s' -o $(BIN_PATH) $(MAIN_PATH)

build: clean build-bin


## docker
docker-setup:
	docker network create -d bridge $(DOCKER_NETWORK)

docker-build: build
	docker build -f ./cmd/gov/Dockerfile -t $(IMAGE_NAME):$(VERSION) -t $(IMAGE_NAME):latest .

docker-run:
	docker run -d --name $(CONTAINER_NAME) \
		-v $$(pwd)/$(CONFIGDIR):/$(CONFIGDIR) \
		-v $$(pwd)/$(PUBLICDIR):/$(PUBLICDIR) \
		-v $$(pwd)/$(TEMPLATEDIR):/$(TEMPLATEDIR) \
		--network=$(DOCKER_NETWORK) \
		-p $(API_PORT):$(API_PORT) $(IMAGE_NAME)

docker-stop:
	if [ "$$(docker ps -q -f name=$(CONTAINER_NAME) -f status=running)" ]; then docker stop $(CONTAINER_NAME); fi
	if [ "$$(docker ps -q -f name=$(CONTAINER_NAME) -f status=exited)" ]; then docker rm $(CONTAINER_NAME); fi

docker-restart: docker-stop docker-run

docker: docker-build docker-restart


## postgres
pg-setup:
	docker volume create --name $(POSTGRES_VOLUME)

pg-run:
	docker run -d --name $(POSTGRES_CONTAINER) --network=$(DOCKER_NETWORK) -p 5432:5432 \
		-v $(POSTGRES_VOLUME):/var/lib/postgresql/data -e POSTGRES_PASSWORD=$(POSTGRES_PASS) postgres:alpine

pg-stop:
	if [ "$$(docker ps -q -f name=$(POSTGRES_CONTAINER) -f status=running)" ]; then docker stop $(POSTGRES_CONTAINER); fi
	if [ "$$(docker ps -q -f name=$(POSTGRES_CONTAINER) -f status=exited)" ]; then docker rm $(POSTGRES_CONTAINER); fi

pg-restart: pg-stop pg-run


## redis
redis-setup:
	docker volume create --name $(REDIS_VOLUME)

redis-run:
	docker run -d --name $(REDIS_CONTAINER) --network=$(DOCKER_NETWORK) -p 6379:6379 -v $(REDIS_VOLUME):/data \
		redis:alpine redis-server --requirepass $(REDIS_PASS)

redis-stop:
	if [ "$$(docker ps -q -f name=$(REDIS_CONTAINER) -f status=running)" ]; then docker stop $(REDIS_CONTAINER); fi
	if [ "$$(docker ps -q -f name=$(REDIS_CONTAINER) -f status=exited)" ]; then docker rm $(REDIS_CONTAINER); fi

redis-restart: redis-stop redis-run


## minio
minio-setup:
	docker volume create --name $(MINIO_VOLUME)
	docker volume create --name $(MINIO_VOLUME_CONFIG)

minio-run:
	docker run -d --name $(MINIO_CONTAINER) --network=$(DOCKER_NETWORK) -p 9000:9000 -v $(MINIO_VOLUME):/export \
		-v $(MINIO_VOLUME_CONFIG):/root/.minio  -e "MINIO_ACCESS_KEY=$(MINIO_KEY)" -e "MINIO_SECRET_KEY=$(MINIO_SECRET)" \
		minio/minio server /export

minio-stop:
	if [ "$$(docker ps -q -f name=$(MINIO_CONTAINER) -f status=running)" ]; then docker stop $(MINIO_CONTAINER); fi
	if [ "$$(docker ps -q -f name=$(MINIO_CONTAINER) -f status=exited)" ]; then docker rm $(MINIO_CONTAINER); fi

minio-restart: minio-stop minio-run


## mailer
mail-run:
	docker run -d --name $(MAIL_CONTAINER) --network=$(DOCKER_NETWORK) -p 1025:1025 -p 8025:8025 mailhog/mailhog

mail-stop:
	if [ "$$(docker ps -q -f name=$(MAIL_CONTAINER) -f status=running)" ]; then docker stop $(MAIL_CONTAINER); fi
	if [ "$$(docker ps -q -f name=$(MAIL_CONTAINER) -f status=exited)" ]; then docker rm $(MAIL_CONTAINER); fi

mail-restart: mail-stop mail-run

## docker dev env
setup: docker-setup pg-setup redis-setup minio-setup

restart: pg-restart redis-restart minio-restart mail-restart

stop: pg-stop redis-stop minio-stop mail-stop
