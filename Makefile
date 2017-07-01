all: build

.PHONY: build

VERSION = $(shell git describe --tags --always --dirty)
BRANCH = $(shell git rev-parse --abbrev-ref HEAD)
REVISION = $(shell git rev-parse HEAD)
REVSHORT = $(shell git rev-parse --short HEAD)
USER = $(shell whoami)

ifneq ($(OS), Windows_NT)
	# If on macOS, set the shell to bash explicitly
	ifeq ($(shell uname), Darwin)
		SHELL := /bin/bash
	endif

	# To populate version metadata, we use unix tools to get certain data
	GOVERSION = $(shell go version | awk '{print $$3}')
	NOW	= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
else
	# To populate version metadata, we use windows tools to get the certain data
	GOVERSION_CMD = "(go version).Split()[2]"
	GOVERSION = $(shell powershell $(GOVERSION_CMD))
	NOW	= $(shell powershell Get-Date -format s)
endif


build: launcher extension

.pre-build:
	mkdir -p build

extension: .pre-build
	go build -i -o build/osquery-extension.ext ./cmd/osquery-extension/

launcher: .pre-build
	go build -i -o build/launcher -ldflags "\
	-X github.com/kolide/launcher/vendor/github.com/kolide/kit/version.appName=launcher \
	-X github.com/kolide/launcher/vendor/github.com/kolide/kit/version.version=${VERSION} \
	-X github.com/kolide/launcher/vendor/github.com/kolide/kit/version.branch=${BRANCH} \
	-X github.com/kolide/launcher/vendor/github.com/kolide/kit/version.revision=${REVISION} \
	-X github.com/kolide/launcher/vendor/github.com/kolide/kit/version.buildDate=${NOW} \
	-X github.com/kolide/launcher/vendor/github.com/kolide/kit/version.buildUser=${USER} \
	-X github.com/kolide/launcher/vendor/github.com/kolide/kit/version.goVersion=${GOVERSION}" ./cmd/launcher/

mac-pkg-builder: .pre-build
	go build -i -o build/mac-pkg-builder ./cmd/mac-pkg-builder/

deps:
	go get -u github.com/golang/dep/cmd/dep
	dep ensure -v

test:
	go test -cover -v $(shell go list ./... | grep -v /vendor/)

build-mac-pkg: launcher extension mac-pkg-builder
	mkdir -p bin/
	cp /usr/local/bin/osqueryd ./bin
	cp ./build/launcher ./bin
	cp ./build/osquery-extension.ext ./bin
	./build/mac-pkg-builder -key ${ENROLL_JWT_KEY} -package
	rm -rf bin
