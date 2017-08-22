all: build

.PHONY: build

VERSION = $(shell git describe --tags --always --dirty)
BRANCH = $(shell git rev-parse --abbrev-ref HEAD)
REVISION = $(shell git rev-parse HEAD)
REVSHORT = $(shell git rev-parse --short HEAD)
USER = $(shell whoami)

KIT_VERSION = "\
	-X github.com/kolide/launcher/vendor/github.com/kolide/kit/version.appName=launcher \
	-X github.com/kolide/launcher/vendor/github.com/kolide/kit/version.version=${VERSION} \
	-X github.com/kolide/launcher/vendor/github.com/kolide/kit/version.branch=${BRANCH} \
	-X github.com/kolide/launcher/vendor/github.com/kolide/kit/version.revision=${REVISION} \
	-X github.com/kolide/launcher/vendor/github.com/kolide/kit/version.buildDate=${NOW} \
	-X github.com/kolide/launcher/vendor/github.com/kolide/kit/version.buildUser=${USER} \
	-X github.com/kolide/launcher/vendor/github.com/kolide/kit/version.goVersion=${GOVERSION}"

ifneq ($(OS), Windows_NT)
	CURRENT_PLATFORM = linux

	# If on macOS, set the shell to bash explicitly
	ifeq ($(shell uname), Darwin)
		SHELL := /bin/bash
		CURRENT_PLATFORM = darwin
	endif

	# To populate version metadata, we use unix tools to get certain data
	GOVERSION = $(shell go version | awk '{print $$3}')
	NOW	= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
else
	CURRENT_PLATFORM = windows
	# To populate version metadata, we use windows tools to get the certain data
	GOVERSION_CMD = "(go version).Split()[2]"
	GOVERSION = $(shell powershell $(GOVERSION_CMD))
	NOW	= $(shell powershell Get-Date -format s)
endif

build: launcher extension

.pre-build:
	mkdir -p build/darwin
	mkdir -p build/linux

extension: .pre-build
	go build -i -o build/osquery-extension.ext ./cmd/osquery-extension/

xp-extension: .pre-build
	GOOS=darwin go build -i -o build/darwin/osquery-extension.ext ./cmd/osquery-extension/
	GOOS=linux CGO_ENABLED=0 go build -i -o build/linux/osquery-extension.ext ./cmd/osquery-extension/
	ln -f build/$(CURRENT_PLATFORM)/osquery-extension.ext build/osquery-extension.ext

launcher: .pre-build
	go build -i -o build/launcher -ldflags ${KIT_VERSION} ./cmd/launcher/

xp-launcher: .pre-build
	GOOS=darwin go build -i -o build/darwin/launcher -ldflags ${KIT_VERSION} ./cmd/launcher/
	GOOS=linux CGO_ENABLED=0 go build -i -o build/linux/launcher -ldflags ${KIT_VERSION} ./cmd/launcher/
	ln -f build/$(CURRENT_PLATFORM)/launcher build/launcher

package-builder: .pre-build xp-launcher xp-extension
	go build -i -o build/package-builder -ldflags ${KIT_VERSION} ./cmd/package-builder/

.deps:
	go get -u github.com/Masterminds/glide
	go get -u github.com/jteeuwen/go-bindata/...
	glide install

deps: .deps generate

INSECURE ?= false
generate:
	go run ./autoupdate/generate_tuf.go \
		-binary=osqueryd -notary=${NOTARY_URL} -insecure=${INSECURE}
	go run ./autoupdate/generate_tuf.go \
		-binary=launcher -notary=${NOTARY_URL} -insecure=${INSECURE}
	go-bindata -o autoupdate/bindata.go -pkg autoupdate autoupdate/assets/...

test: generate
	go test -cover -race -v $(shell go list ./... | grep -v /vendor/)
