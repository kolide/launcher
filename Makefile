all: build

.PHONY: build

ifndef ($(GOPATH))
	GOPATH = $(HOME)/go
endif

PATH := $(GOPATH)/bin:$(PATH)

export GO111MODULE=on
export GOPROXY=https://proxy.golang.org
export GOSUMDB=sum.golang.org

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

xp-%:
	$(MAKE) -j darwin-xp-$* windows-xp-$* linux-xp-$*

darwin-xp-%: .pre-build
	go run cmd/make/make.go -targets=$* -linkstamp -os=darwin

linux-xp-%: .pre-build
	go run cmd/make/make.go -targets=$* -linkstamp -os=linux

windows-xp-%: .pre-build
	go run cmd/make/make.go -targets=$* -linkstamp -os=windows


codesign-darwin: xp
	codesign --force -s "${CODESIGN_IDENTITY}" -v ./build/darwin/launcher
	codesign --force -s "${CODESIGN_IDENTITY}" -v ./build/darwin/osquery-extension.ext

codesign: codesign-darwin

package-builder: .pre-build deps
	go run cmd/make/make.go -targets=package-builder -linkstamp

package-builder-windows: .pre-build deps
	go run cmd/make/make.go -targets=package-builder -linkstamp --os windows
launcher-pummel:
	go run cmd/make/make.go -targets=launcher-pummel

deps-go:
	go run cmd/make/make.go -targets=deps-go,install-tools

deps: deps-go generate

generate: deps-go
	-find /go -type d
	-find /root/go -type d
	go generate ./pkg/packagekit
	go run cmd/make/make.go -targets=generate-tuf

proto:
	@(cd pkg/pb/launcher; go generate)
	@(cd pkg/pb/querytarget; go generate)
	@echo "Generated code from proto definitions."

test: generate
	go test -cover -race -v $(shell go list ./... | grep -v /vendor/)

##
## Lint
##

# These are escape newlines, looks super weird. Allows these to run in
# parallel with `make -j`
lint: \
	lint-go-deadcode \
	lint-misspell \
	lint-go-vet \
	lint-go-nakedret \
	lint-go-fmt

lint-go-deadcode: deps-go
	deadcode cmd/ pkg/

lint-misspell: deps-go
	git ls-files \
	  | grep -v pkg/simulator/testdata/bad_symlink \
	  | xargs misspell -error -f 'misspell: {{ .Filename }}:{{ .Line }}:{{ .Column }}:corrected {{ printf "%q" .Original }} to {{ printf "%q" .Corrected }}'

lint-go-vet:
	go vet ./cmd/... ./pkg/...

lint-go-nakedret: deps-go
	nakedret ./...

# This is a big ugly, since go-fmt doesn't have a simple exit code. Thus the doubled echo and test.
lint-go-fmt: export FMTFAILS = $(shell gofmt -l ./pkg/ ./cmd/ | grep -vE 'assets.go|bindata.go')
lint-go-fmt: deps-go
	@test -z "$(FMTFAILS)" || echo gofmt failures in: "$(FMTFAILS)"
	@test -z "$(FMTFAILS)"

##
## Release Process Stuff
##

release: binary-bundle containers-push

binary-bundle: VERSION = $(shell git describe --tags --always --dirty)
binary-bundle: codesign
	rm -rf build/binary-bundle
	$(MAKE) -j $(foreach p, darwin linux windows, build/binary-bundle/$(p))
	cd build/binary-bundle && zip -r "launcher_${VERSION}.zip" *

build/binary-bundle/%:
	mkdir -p $@
	cp build/$*/launcher* $@/
	cp build/$*/osquery-extension* $@/
	go run ./tools/download-osquery.go --platform=$* --output=$@/osqueryd

##
## Docker Tooling
##

CONTAINER_OSES = ubuntu16 ubuntu18 centos6 centos7 distroless

.PHONY: containers
containers: $(foreach c,$(CONTAINER_OSES),docker-$(c) dockerfake-$(c))
containers-push: $(foreach c,$(CONTAINER_OSES),dockerpush-$(c) dockerpush-fakedata-$(c))

build-docker:
	docker build -t launcher-build  .

build-dockerfake:
	docker build -t launcher-fakedata-build --build-arg FAKE=-fakedata .

dockerfake-%:  build-dockerfake
	docker build -t gcr.io/kolide-public-containers/launcher-fakedata-$* --build-arg FAKE=-fakedata docker/$*

docker-%: build-docker
	docker build -t gcr.io/kolide-public-containers/launcher-$*  docker/$*

dockerpush-%: docker-%
	docker push gcr.io/kolide-public-containers/launcher-$*



# Porter is a kolide tool to update notary, part of the update framework
porter-%: codesign
	for p in darwin linux windows; do \
	  echo porter mirror -debug -channel $* -platform $$p -launcher-all; \
	  echo porter mirror -debug -channel $* -platform $$p -extension-tarball -extension-upload; \
	done
