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

export GO111MODULE=on

KIT_VERSION = "\
	-X github.com/kolide/kit/version.appName=${APP_NAME} \
	-X github.com/kolide/kit/version.version=${VERSION} \
	-X github.com/kolide/kit/version.branch=${BRANCH} \
	-X github.com/kolide/kit/version.revision=${REVISION} \
	-X github.com/kolide/kit/version.buildDate=${NOW} \
	-X github.com/kolide/kit/version.buildUser=${USER} \
	-X github.com/kolide/kit/version.goVersion=${GOVERSION}"

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

.pre-build: ${BUILD_DIR}

${BUILD_DIR}:
ifeq ($(OS), Windows_NT)
	powershell New-Item -Type Directory -Force -Path ${BUILD_DIR} | powershell Out-Null
else
	mkdir -p ${BUILD_DIR}
endif

extension: .pre-build
	go build -o build/osquery-extension.ext ./cmd/osquery-extension/

osqueryi: .pre-build
	go build -o build/launcher.ext ./cmd/launcher.ext/
	osqueryi --extension=./build/launcher.ext

xp-extension: .pre-build
	GOOS=darwin go build -o build/darwin/osquery-extension.ext ./cmd/osquery-extension/
	GOOS=linux CGO_ENABLED=0 go build -o build/linux/osquery-extension.ext ./cmd/osquery-extension/
	ln -f build/$(CURRENT_PLATFORM)/osquery-extension.ext build/osquery-extension.ext

.pre-launcher:
	$(eval APP_NAME = launcher)

launcher: .pre-build .pre-launcher
	go build -o build/launcher -ldflags ${KIT_VERSION} ./cmd/launcher/

codesign-darwin:
	codesign --force -s "${CODESIGN_IDENTITY}" -v ./build/darwin/launcher
	codesign --force -s "${CODESIGN_IDENTITY}" -v ./build/darwin/osquery-extension.ext

xp: xp-launcher xp-extension

xp-codesign: xp codesign-darwin

xp-launcher: .pre-build .pre-launcher
	GOOS=darwin go build -o build/darwin/launcher -ldflags ${KIT_VERSION} ./cmd/launcher/
	GOOS=linux CGO_ENABLED=0 go build -o build/linux/launcher -ldflags ${KIT_VERSION} ./cmd/launcher/
	ln -f build/$(CURRENT_PLATFORM)/launcher build/launcher

.pre-package-builder:
	$(eval APP_NAME = package-builder)

package-builder: .pre-build xp-codesign .pre-package-builder generate
	go build -o build/package-builder -ldflags ${KIT_VERSION} ./cmd/package-builder/

.pre-launcher-pummel:
	$(eval APP_NAME = launcher-pummel)

launcher-pummel:
	go build -o build/launcher-pummel -ldflags ${KIT_VERSION} ./cmd/launcher-pummel/

deps-go: 
	go run cmd/make/make.go -targets=deps-go,install-tools

deps: deps-go generate

generate:
	go run cmd/make/make.go -targets=generate-tuf

proto:
	@(cd pkg/pb/launcher; go generate)
	@echo "Generated code from proto definitions."

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

builder:
	cd tools/builders/launcher-builder/1.11/ && gcloud builds submit --project=kolide-public-containers --config=cloudbuild.yml

binary-bundle: xp-codesign
	rm -rf build/binary-bundle
	mkdir -p build/binary-bundle/linux
	mkdir -p build/binary-bundle/darwin
	cp build/linux/launcher build/binary-bundle/linux/launcher
	cp build/linux/osquery-extension.ext build/binary-bundle/linux/osquery-extension.ext
	go run ./tools/download-osquery.go --platform=linux --output=build/binary-bundle/linux/osqueryd
	cp build/darwin/launcher build/binary-bundle/darwin/launcher
	cp build/darwin/osquery-extension.ext build/binary-bundle/darwin/osquery-extension.ext
	go run ./tools/download-osquery.go --platform=darwin --output=build/binary-bundle/darwin/osqueryd
	cd build/binary-bundle && zip -r "launcher_${VERSION}.zip" linux/ darwin/
	cp build/binary-bundle/launcher_${VERSION}.zip build/binary-bundle/launcher_latest.zip
