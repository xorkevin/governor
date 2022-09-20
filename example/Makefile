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
ANVIL_REGISTER=$(ANVIL_OUT)/secrets/register/governor
ANVIL_BASE=$(ANVIL_OUT)/governor
ANVIL_NS=$(ANVIL_BASE)/namespace.yaml

.PHONY: devgen devsecrets devregister devunregister devinit devconfig

devgen: ## Generate k8s resources
	if [ -d $(ANVIL_OUT) ]; then rm -r $(ANVIL_OUT); fi
	anvil component -i $(ANVIL_SRC) -o $(ANVIL_OUT) -c $(ANVIL_MAIN)

devsecrets: ## Register vault resources
	@VAULT_ADDR=$(VAULT_ADDR) VAULT_TOKEN=$(VAULT_TOKEN) anvil secret --vault-policies=$(ANVIL_POLICIES) --vault-roles=$(ANVIL_ROLES)

devregister: ## Register k8s resources
	kubectl apply -f $(ANVIL_NS)
	kubectl apply -k $(ANVIL_REGISTER)

devunregister: ## Unregister k8s resources
	kubectl delete -k $(ANVIL_REGISTER)

devinit: devgen devsecrets devregister ## Init resources

devconfig: ## Set app config
	./setsmtpauth.sh

.PHONY: devup devdown devnsdown

devup: ## Deploy k8s resources to cluster
	kubectl apply -k $(ANVIL_OUT)/governor

devdown: ## Destroy k8s resources
	kubectl delete -k $(ANVIL_OUT)/governor

devnsdown: ## Destroy k8s namespace
	kubectl delete -f $(ANVIL_NS)

## DOCKER COMPOSE DEV ENV

DC_ANVIL_SRC=dc.anvil
DC_ANVIL_MAIN=$(DC_ANVIL_SRC)/component.yaml
DC_ANVIL_OUT=dc.anvil_out
DC_PROJECT=governor

.PHONY: dc-devgen dc-devup dc-devdown

dc-devgen: ## Generate docker compose resources
	if [ -d $(DC_ANVIL_OUT) ]; then rm -r $(DC_ANVIL_OUT); fi
	anvil component -i $(DC_ANVIL_SRC) -o $(DC_ANVIL_OUT) -c $(DC_ANVIL_MAIN)

dc-devup: ## Deploy docker-compose resources
	docker-compose -p $(DC_PROJECT) -f $(DC_ANVIL_OUT)/governor/docker-compose.yaml up -d

dc-devdown: ## Destroy docker-compose resources
	docker-compose -p $(DC_PROJECT) -f $(DC_ANVIL_OUT)/governor/docker-compose.yaml down

## APP BUILD

BIN_NAME=governor
MAIN_PATH=./cmd
BIN_DIR=./bin
BIN_PATH=$(BIN_DIR)/$(BIN_NAME)

.PHONY: clean build buildrace

clean: ## Remove build artifacts
	if [ -d $(BIN_DIR) ]; then rm -r $(BIN_DIR); fi

build: ## Build app
	mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 go build -trimpath -ldflags "-w -s" -o $(BIN_PATH) $(MAIN_PATH)

buildrace: ## Build app with data race detection
	mkdir -p $(BIN_DIR)
	CGO_ENABLED=1 go build -race -trimpath -ldflags "-w -s" -o $(BIN_PATH) $(MAIN_PATH)

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

.PHONY: dev devstop tail setupfirst setup

dev: build ## Deploy app to cluster
	skaffold run

devstop: ## Stop running app
	skaffold delete

NAMESPACE=governor
DEVDEPLOY=governor
DEVPOD=$(shell kubectl -n $(NAMESPACE) get pods -o=name --sort-by=.metadata.creationTimestamp | grep pod/governor | head -n 1)
DEVCONTAINER=governor
TAILLINES?=64

tail: ## Tail log lines
	kubectl -n $(NAMESPACE) logs -f --tail=$(TAILLINES) $(DEVPOD) $(DEVCONTAINER)

setupfirst: build ## Run app first setup
	$(BIN_PATH) setup --first

setup: build ## Run app setup
	./setup.sh "$(BIN_PATH)"

## LOCAL DEV

.PHONY: serve run runrace runsetup

serve: ## Serves app
	$(BIN_PATH) serve --config $(DC_ANVIL_OUT)/governor/config.yaml

run: build serve ## Runs app locally

runrace: buildrace serve ## Runs app locally with data race detection

runsetup: build ## Runs setup locally
	$(BIN_PATH) setup --secret "setupsecret"
