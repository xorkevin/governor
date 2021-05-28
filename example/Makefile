## PROLOG

.PHONY: help all

CMDNAME=governor
CMDDESC=example project

help: ## Print this help
	@./help.sh '$(CMDNAME)' '$(CMDDESC)'

all: build ## Default

## DEV ENV

ANVIL_SRC=anvil
ANVIL_MAIN=$(ANVIL_SRC)/component.yaml
ANVIL_OUT=anvil_out
ANVIL_POLICIES=$(ANVIL_OUT)/secrets/vault/policy
ANVIL_ROLES=$(ANVIL_OUT)/secrets/vault/roles
VAULT_ADDR=http://vault.dev.localhost:8080/
VAULT_TOKEN=$(shell cat ~/.vault-token)
ANVIL_REGISTER=$(ANVIL_OUT)/secrets/register
ANVIL_NS=$(ANVIL_OUT)/governor/namespace.yaml

.PHONY: devgen devsecrets devregister devunregister devinit

devgen: ## Generate k8s resources
	if [ -d $(ANVIL_OUT) ]; then rm -r $(ANVIL_OUT); fi
	anvil component -i $(ANVIL_SRC) -o $(ANVIL_OUT) -c $(ANVIL_MAIN)

devsecrets: ## Register vault resources
	@VAULT_ADDR=$(VAULT_ADDR) VAULT_TOKEN=$(VAULT_TOKEN) anvil secret --vault-policies=$(ANVIL_POLICIES) --vault-roles=$(ANVIL_ROLES)

devregister: ## Register k8s resources
	kubectl apply -f $(ANVIL_NS)
	kubectl apply -k $(ANVIL_REGISTER)/governor/postgres

devunregister: ## Unregister k8s resources
	kubectl delete -k $(ANVIL_REGISTER)/governor/postgres

devinit: devgen devsecrets devregister ## Init resources

.PHONY: devup devinit2 devconfig devdown

devup: ## Deploy k8s resources to cluster
	kubectl apply -k base

devinit2: ## Register k8s resources post deploy
	./register2.sh

devconfig: ## Set app config
	./setsmtpauth.sh

devdown: ## Destroy k8s resources
	kubectl delete -k base

## APP BUILD

BIN_NAME=governor
MAIN_PATH=cmd/main.go
BIN_DIR=./bin
BIN_PATH=$(BIN_DIR)/$(BIN_NAME)

.PHONY: clean build

clean: ## Remove build artifacts
	if [ -d $(BIN_DIR) ]; then rm -r $(BIN_DIR); fi

build: ## Build app
	mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 go build -trimpath -ldflags "-w -s -X main.GitHash=$$(git rev-parse --verify HEAD)" -o $(BIN_PATH) $(MAIN_PATH)

## DOCKER

DOCKER_NAME=docker.pkg.dev.localhost:8080/governor

DOCKER_MAJOR_VERSION=0.2
DOCKER_VERSION=0.2.7-0

DOCKER_LATEST_IMAGE=$(DOCKER_NAME):latest
DOCKER_MAJOR_IMAGE=$(DOCKER_NAME):$(DOCKER_MAJOR_VERSION)
DOCKER_IMAGE=$(DOCKER_NAME):$(DOCKER_VERSION)

.PHONY: build-docker publish-docker docker

build-docker: build ## Build docker image
	docker build -t $(DOCKER_IMAGE) -t $(DOCKER_MAJOR_IMAGE) -t $(DOCKER_LATEST_IMAGE) .

publish-docker: ## Publish docker image
	docker push $(DOCKER_IMAGE)
	docker push $(DOCKER_MAJOR_IMAGE)
	docker push $(DOCKER_LATEST_IMAGE)

docker: build-docker publish-docker ## Release new image

## APP DEPLOY

.PHONY: skaffoldinit

skaffoldinit: ## Initialize skaffold
	skaffold config set -g update-check false

.PHONY: dev devstop tail setup

dev: build ## Deploy app to cluster
	skaffold run

devstop: ## Stop running app
	skaffold delete

NAMESPACE=governor
DEVDEPLOY=governor
DEVPOD=$(shell kubectl -n $(NAMESPACE) get pods -o=name --sort-by=.metadata.creationTimestamp | grep governor | head -n 1)
DEVCONTAINER=governor
TAILLINES?=64

tail: ## Tail log lines
	kubectl -n $(NAMESPACE) logs -f --tail=$(TAILLINES) $(DEVPOD) $(DEVCONTAINER)

setup: build ## Run app setup
	$(BIN_PATH) setup
