all: build

.PHONY: build

ifndef ($(GOPATH))
	GOPATH = $(HOME)/go
endif

PATH := $(GOPATH)/bin:$(PATH)
VERSION = $(shell git describe --tags --always --dirty)
BRANCH = $(shell git rev-parse --abbrev-ref HEAD)
REVISION = $(shell git rev-parse HEAD)
REVSHORT = $(shell git rev-parse --short HEAD)
USER = $(shell whoami)

KIT_VERSION = "\
	-X github.com/kolide/launcher/vendor/github.com/kolide/kit/version.appName=${APP_NAME} \
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

osqueryi: .pre-build
	go build -i -o build/development-extension.ext ./cmd/development-extension/
	osqueryi --extension=./build/development-extension.ext

xp-extension: .pre-build
	GOOS=darwin go build -i -o build/darwin/osquery-extension.ext ./cmd/osquery-extension/
	GOOS=linux CGO_ENABLED=0 go build -i -o build/linux/osquery-extension.ext ./cmd/osquery-extension/
	ln -f build/$(CURRENT_PLATFORM)/osquery-extension.ext build/osquery-extension.ext

.pre-launcher:
	$(eval APP_NAME = launcher)

launcher: .pre-build .pre-launcher
	go build -i -o build/launcher -ldflags ${KIT_VERSION} ./cmd/launcher/

xp-launcher: .pre-build .pre-launcher
	GOOS=darwin go build -i -o build/darwin/launcher -ldflags ${KIT_VERSION} ./cmd/launcher/
	GOOS=linux CGO_ENABLED=0 go build -i -o build/linux/launcher -ldflags ${KIT_VERSION} ./cmd/launcher/
	ln -f build/$(CURRENT_PLATFORM)/launcher build/launcher

.pre-package-builder:
	$(eval APP_NAME = package-builder)

package-builder: .pre-build xp-launcher xp-extension .pre-package-builder generate
	go build -i -o build/package-builder -ldflags ${KIT_VERSION} ./cmd/package-builder/

.deps:
	go get -u github.com/Masterminds/glide
	go get -u github.com/jteeuwen/go-bindata/...
	glide install

deps: .deps generate

# First, we generate a bindata file from an empty directory so that the symbols
# are present (Asset, AssetDir, etc). Once the symbols are present, we can run
# the generate_tuf.go tool to generate actual TUF metadata. Finally, we recreate
# the bindata file with the real TUF metadata.
generate:
	$(eval EMPTY_BINDATA_DIR = $(shell mktemp -d))
	go-bindata \
		-o autoupdate/bindata.go \
		-pkg autoupdate \
		$(EMPTY_BINDATA_DIR)
	go run ./tools/notary/generate_tuf.go -binary osqueryd
	go run ./tools/notary/generate_tuf.go -binary launcher
	go-bindata \
		-o autoupdate/bindata.go \
		-pkg autoupdate \
		autoupdate/assets/...

# Publishes osqueryd for autoupdate. NOTARY_DELEGATE_PASSPHRASE must be set
# and the delegate key must be imported by Notary client.
publish-osquery: package-builder
	./build/package-builder mirror -osquery-all -platform darwin
	./build/package-builder mirror -osquery-all -platform linux

# Publishes launcher for autoupdate. NOTARY_DELEGATE_PASSPHRASE must be set
# and the delegate key must be imported by Notary client.
publish-launcher: package-builder
	./build/package-builder mirror -launcher-all -platform darwin
	./build/package-builder mirror -launcher-all -platform linux

# Publishes launcher and osqueryd for autoupdate. NOTARY_DELEGATE_PASSPHRASE must be set
# and the delegate key must be imported by Notary client.
publish: package-builder
	./build/package-builder mirror -all -platform darwin
	./build/package-builder mirror -all -platform linux

test: generate
	go test -cover -race -v $(shell go list ./... | grep -v /vendor/)

install: build
	mkdir -p $(GOPATH)/bin
	cp ./build/launcher $(GOPATH)/bin/launcher
	cp ./build/osquery-extension.ext $(GOPATH)/bin/osquery-extension.ext

CONTAINERS = ubuntu14 ubuntu16 centos6 centos7

.PHONY: push-containers containers $(CONTAINERS)

containers: $(CONTAINERS)

$(CONTAINERS): xp-launcher xp-extension
	docker build -t gcr.io/kolide-ose-testing/${@}-launcher:latest -f docker/${@}/Dockerfile .
	VERSION=$$(docker run --rm gcr.io/kolide-ose-testing/${@}-launcher:latest launcher -version | head -1 | sed 's/launcher - version //g')
	docker tag gcr.io/kolide-ose-testing/${@}-launcher:latest gcr.io/kolide-ose-testing/${@}-launcher:${VERSION}

push-containers: $(CONTAINERS)
	for container in $(CONTAINERS); do \
		gcloud docker -- push gcr.io/kolide-ose-testing/$${container}-launcher; \
	done
