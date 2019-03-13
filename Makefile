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

codesign: xp codesign-darwin

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

binary-bundle: VERSION = $(shell git describe --tags --always --dirty)
binary-bundle: codesign
	rm -rf build/binary-bundle
	$(MAKE) -j $(foreach p, darwin linux windows, build/binary-bundle/$(p))
	cd build/binary-bundle && zip -r "launcher_${VERSION}.zip" *
	cp build/binary-bundle/launcher_${VERSION}.zip build/binary-bundle/launcher_latest.zip

build/binary-bundle/%:
	mkdir -p $@
	cp build/$*/launcher* $@/
	cp build/$*/osquery-extension* $@/
	go run ./tools/download-osquery.go --platform=$* --output=$@/osqueryd

# These are escape newlines, looks super weird. Allows these to run in
# parallel with `make -j`
lint: \
	lint-go-deadcode \
	lint-misspell \
	lint-go-vet \
	lint-go-nakedret

lint-go-deadcode:
	deadcode cmd/ pkg/

lint-misspell:
	git ls-files \
	  | grep -v pkg/simulator/testdata/bad_symlink \
	  | xargs misspell -error -f 'misspell: {{ .Filename }}:{{ .Line }}:{{ .Column }}:corrected {{ printf "%q" .Original }} to {{ printf "%q" .Corrected }}'

lint-go-vet:
	go vet ./cmd/... ./pkg/...

lint-go-nakedret:
	nakedret ./...


builder:
	cd tools/builders/launcher-builder/1.11/ && gcloud builds submit --project=kolide-public-containers --config=cloudbuild.yml


##
## Docker Tooling
##

CONTAINER_OSES = ubuntu16 ubuntu18 centos6 centos7

.PHONY: containers
containers: $(foreach c,$(CONTAINER_OSES),docker-$(c) dockerfake-$(c))
containers-push: $(foreach c,$(CONTAINER_OSES),dockerpush-$(c) dockerpush-fakedata-$(c))

docker-build:
	docker build -t launcher-fakedata-build --build-arg FAKE=-fakedata .
	docker build -t launcher-build .

dockerfake-%:
	docker build -t gcr.io/kolide-public-containers/launcher-fakedata-$* --build-arg FAKE=-fakedata docker/$*

docker-%:
	docker build -t gcr.io/kolide-public-containers/launcher-$*  docker/$*

dockerpush-%:
	docker push gcr.io/kolide-public-containers/launcher-$*
