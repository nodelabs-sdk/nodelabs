#!/usr/bin/make -f

BRANCH := $(shell git rev-parse --abbrev-ref HEAD 2>/dev/null || echo "unknown")
COMMIT := $(shell git log -1 --format='%H' 2>/dev/null || echo "unknown")
BUILDDIR ?= $(CURDIR)/build

# Build tags
build_tags = netgo
build_tags += $(BUILD_TAGS)
build_tags := $(strip $(build_tags))

# Linker flags
ldflags = -X github.com/cosmos/cosmos-sdk/version.Name=nodelabs \
	-X github.com/cosmos/cosmos-sdk/version.AppName=nodelabsd \
	-X github.com/cosmos/cosmos-sdk/version.Version=$(shell git describe --tags --always 2>/dev/null || echo "dev") \
	-X github.com/cosmos/cosmos-sdk/version.Commit=$(COMMIT)

BUILD_FLAGS := -tags "$(build_tags)" -ldflags '$(ldflags)'

###############################################################################
###                                Build                                    ###
###############################################################################

all: build

build: go.sum
	@echo "--> Building nodelabsd"
	@CGO_ENABLED=1 go build -mod=readonly $(BUILD_FLAGS) -o $(BUILDDIR)/nodelabsd ./cmd/nodelabsd

install: go.sum
	@echo "--> Installing nodelabsd"
	@CGO_ENABLED=1 go install -mod=readonly $(BUILD_FLAGS) ./cmd/nodelabsd

go.sum: go.mod
	@echo "--> Ensuring dependencies have not been modified"
	@go mod verify
	@go mod tidy

###############################################################################
###                                Testing                                  ###
###############################################################################

test:
	@echo "--> Running tests"
	@go test -race ./...

###############################################################################
###                                Linting                                  ###
###############################################################################

lint:
	@echo "--> Running linter"
	@golangci-lint run --timeout=10m

###############################################################################
###                                Protobuf                                 ###
###############################################################################

DOCKER := $(shell which docker)
CURRENT_UID := $(shell id -u)
CURRENT_GID := $(shell id -g)

protoVer=0.13.2
protoImageName=ghcr.io/cosmos/proto-builder:$(protoVer)
protoImage="$(DOCKER)" run -e BUF_CACHE_DIR=/tmp/buf --rm -v "$(CURDIR)":/workspace:rw --user $(CURRENT_UID):$(CURRENT_GID) --workdir /workspace $(protoImageName)

proto-all: proto-format proto-lint proto-gen format

proto-gen:
	@echo "Generating Protobuf files"
	@$(protoImage) sh ./scripts/protocgen.sh
	@spawn stub-gen
	@go mod tidy

proto-format:
	@echo "Formatting Protobuf files"
	@$(protoImage) find ./ -name "*.proto" -exec clang-format -i {} \;

proto-lint:
	@echo "Linting Protobuf files"
	@$(protoImage) sh -c "cd proto && buf lint --error-format=json"

###############################################################################
###                            Local Testnet                                ###
###############################################################################

sh-testnet: install
	@./scripts/testnet.sh -y

.PHONY: all build install test lint proto-all proto-gen proto-format proto-lint sh-testnet
