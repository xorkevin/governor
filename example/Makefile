## PROLOG

.PHONY: help all

CMDNAME=governor
CMDDESC=example project

help: ## Print this help
	@./help.sh '$(CMDNAME)' '$(CMDDESC)'

all: build ## Default

## KUBE DEV ENV

KUBE_ANVIL_SRC=anvil/kube
KUBE_ANVIL_MAIN=$(KUBE_ANVIL_SRC)/component.yaml
KUBE_ANVIL_OUT=anvil_out
KUBE_ANVIL_POLICIES=$(KUBE_ANVIL_OUT)/secrets/vault/policy
KUBE_ANVIL_ROLES=$(KUBE_ANVIL_OUT)/secrets/vault/roles
KUBE_VAULT_ADDR=http://vault.dev.localhost:8080/
KUBE_VAULT_TOKEN=$(shell cat ~/.vault-token)
KUBE_ANVIL_REGISTER=$(KUBE_ANVIL_OUT)/secrets/register/governor
KUBE_ANVIL_BASE=$(KUBE_ANVIL_OUT)/governor
KUBE_ANVIL_NS=$(KUBE_ANVIL_BASE)/namespace.yaml

.PHONY: kubegen kubesecrets kuberegister kubeunregister kubeinit kubeconfig

kubegen: ## Generate k8s resources
	if [ -d $(KUBE_ANVIL_OUT) ]; then rm -r $(KUBE_ANVIL_OUT); fi
	anvil component -i $(KUBE_ANVIL_SRC) -o $(KUBE_ANVIL_OUT) -c $(KUBE_ANVIL_MAIN)

kubesecrets: ## Register vault resources
	@VAULT_ADDR=$(KUBE_VAULT_ADDR) VAULT_TOKEN=$(KUBE_VAULT_TOKEN) anvil secret --vault-policies=$(KUBE_ANVIL_POLICIES) --vault-roles=$(KUBE_ANVIL_ROLES)

kuberegister: ## Register k8s resources
	kubectl apply -f $(KUBE_ANVIL_NS)
	kubectl apply -k $(KUBE_ANVIL_REGISTER)

kubeunregister: ## Unregister k8s resources
	kubectl delete -k $(KUBE_ANVIL_REGISTER)

kubeinit: kubegen kubesecrets kuberegister ## Init resources

kubeconfig: ## Set app config
	./setsmtpauth.sh

.PHONY: kubeup kubedown kubensdown

kubeup: ## Deploy k8s resources to cluster
	kubectl apply -k $(KUBE_ANVIL_OUT)/governor

kubedown: ## Destroy k8s resources
	kubectl delete -k $(KUBE_ANVIL_OUT)/governor

kubensdown: ## Destroy k8s namespace
	kubectl delete -f $(KUBE_ANVIL_NS)

## DOCKER COMPOSE DEV ENV

ANVIL_COMPOSE_INP=compose/anvil/main.jsonnet
ANVIL_COMPOSE_OUT=compose/anvil_out

.PHONY: devgen devup devdown

devgen: ## Generate docker compose resources
	if [ -d $(ANVIL_COMPOSE_OUT) ]; then rm -r $(ANVIL_COMPOSE_OUT); fi
	anvil component --input $(ANVIL_COMPOSE_INP) --output $(ANVIL_COMPOSE_OUT)

devup: ## Deploy docker compose resources
	docker compose --file $(ANVIL_COMPOSE_OUT)/governor/compose.yaml up --detach --wait

devdown: ## Destroy docker compose resources
	docker compose --file $(ANVIL_COMPOSE_OUT)/governor/compose.yaml down

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
	$(BIN_PATH) serve --config $(ANVIL_COMPOSE_OUT)/governor/config.yaml

run: build serve ## Runs app locally

runrace: buildrace serve ## Runs app locally with data race detection

runsetup: build ## Runs setup locally
	$(BIN_PATH) setup --secret "admin"
