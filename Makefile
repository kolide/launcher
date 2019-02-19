all: build

.PHONY: build

ifndef ($(GOPATH))
	GOPATH = $(HOME)/go
endif

PATH := $(GOPATH)/bin:$(PATH)

export GO111MODULE=on

# If on macOS, set the shell to bash explicitly
ifneq ($(OS), Windows_NT)
	ifeq ($(shell uname), Darwin)
		SHELL := /bin/bash
	endif
endif

all: build
build: launcher extension
.pre-build: ${BUILD_DIR}

${BUILD_DIR}:
ifeq ($(OS), Windows_NT)
	powershell New-Item -Type Directory -Force -Path ${BUILD_DIR} | powershell Out-Null
else
	mkdir -p ${BUILD_DIR}
endif

launcher: .pre-build
	go run cmd/make/make.go -targets=launcher -linkstamp

table.ext: .pre-build
	go run cmd/make/make.go -targets=table-extension -linkstamp

osqueryi-tables: table.ext
	osqueryd -S --extension ./build/darwin/tables.ext

extension: .pre-build
	go run cmd/make/make.go -targets=extension

xp: xp-launcher xp-extension xp-grpc-extension

xp-%: .pre-build
	go run cmd/make/make.go -targets=$* -linkstamp -os=darwin
	go run cmd/make/make.go -targets=$* -linkstamp -os=linux
	go run cmd/make/make.go -targets=$* -linkstamp -os=windows


codesign-darwin:
	codesign --force -s "${CODESIGN_IDENTITY}" -v ./build/darwin/launcher
	codesign --force -s "${CODESIGN_IDENTITY}" -v ./build/darwin/osquery-extension.ext

xp-codesign: xp codesign-darwin

package-builder: .pre-build deps
	go run cmd/make/make.go -targets=package-builder -linkstamp

package-builder-windows: .pre-build deps
	go run cmd/make/make.go -targets=package-builder -linkstamp --os windows
launcher-pummel:
	go run cmd/make/make.go -targets=launcher-pummel

deps-go:
	go run cmd/make/make.go -targets=deps-go,install-tools

deps: deps-go generate

generate:
	go generate ./pkg/packagekit
	go run cmd/make/make.go -targets=generate-tuf

proto:
	@(cd pkg/pb/launcher; go generate)
	@(cd pkg/pb/querytarget; go generate)
	@echo "Generated code from proto definitions."

test: generate
	go test -cover -race -v $(shell go list ./... | grep -v /vendor/)

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

binary-bundle: VERSION = $(shell git describe --tags --always --dirty)
binary-bundle: xp-codesign
	rm -rf build/binary-bundle
	$(MAKE) -j $(foreach p, darwin linux windows, build/binary-bundle/$(p))
	cd build/binary-bundle && zip -r "launcher_${VERSION}.zip" *
	cp build/binary-bundle/launcher_${VERSION}.zip build/binary-bundle/launcher_latest.zip

build/binary-bundle/%:
	mkdir -p $@
	cp build/$*/launcher* $@/
	cp build/$*/osquery-extension* $@/
	go run ./tools/download-osquery.go --platform=$* --output=$@/osqueryd
