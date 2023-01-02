all: help

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk commands is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Developement

fmt: ## Run go fmt against code.
	go fmt ./...

vet: ## Run go vet against code.
	go vet ./...

run-server: fmt vet ## Run server.
	echo "test" > local_dev.token
	go run . serve --db-dsn ./test.sqlite3?_journal_mode=WAL --metrics-addr :8081 --log-devel --log-level 12 --auth-token-file local_dev.token

run-migrate: fmt vet ## Run migration.
	go run . migrate --db-dsn ./test.sqlite3?_journal_mode=WAL --log-devel --log-level 12 ./data

##@ Build

DOCKER_CMD ?= docker
IMG_TAG ?= latest
IMG_PREFIX ?= ghcr.io/b4fun/sqlite-rest
IMG_BUILD_OPTS ?= --platform=linux/amd64

build: fmt vet ## Build binary.
	go build -o ./sqlite-rest .

build-image: build-image-server ## Build docker images.

build-image-server: ## Build server docker image.
	${DOCKER_CMD} build ${IMG_BUILD_OPTS} \
		-f Dockerfile.server \
		-t ${IMG_PREFIX}/server:${IMG_TAG} .

push-image: ## Push docker images.
	${DOCKER_CMD} push ${IMG_PREFIX}/server:${IMG_TAG}