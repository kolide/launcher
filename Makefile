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

fake-launcher: .pre-build
	go run cmd/make/make.go -targets=launcher -linkstamp -fakedata
	-rm build/darwin/launcher
	mv build/launcher build/launcher-fake

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
	osqueryd -S --allow-unsafe --verbose --extension ./build/darwin/tables.ext
sudo-osqueryi-tables: table.ext
	sudo osqueryd -S --allow-unsafe --verbose --extension ./build/darwin/tables.ext
launchas-osqueryi-tables: table.ext
	sudo launchctl asuser 0 osqueryd -S --allow-unsafe --verbose --extension ./build/darwin/tables.ext


extension: .pre-build
	go run cmd/make/make.go -targets=extension


xp: xp-launcher xp-extension xp-grpc-extension

xp-%: darwin-xp-% windows-xp-% linux-xp-%
	@true # make needs something here for the pattern rule

darwin-xp-%: .pre-build deps
	go run cmd/make/make.go -targets=$* -linkstamp -os=darwin

linux-xp-%: .pre-build deps
	go run cmd/make/make.go -targets=$* -linkstamp -os=linux

windows-xp-%: .pre-build deps
	go run cmd/make/make.go -targets=$* -linkstamp -os=windows



# `-o runtime` should be enough, however there was a catalina bug that
# required we add `library`. This was fixed in 10.15.4. (from
# macadmins slack)
codesign-darwin: xp
	codesign --force -s "${CODESIGN_IDENTITY}" -v --options runtime,library --timestamp ./build/darwin/*

notarize-darwin: codesign-darwin
	rm -f build/notarization-upload.zip
	zip -r build/notarization-upload.zip ./build/darwin/*
	xcrun altool \
	  --username "${NOTARIZE_APPLE_ID}" \
	  --password @env:NOTARIZE_APP_PASSWD \
	  --asc-provider "${NOTARIZE_ACCOUNT_ID}" \
	  --notarize-app --file build/notarization-upload.zip \
	  --primary-bundle-id com.kolide.launcher

# notarize-check is a helper for checking uuids
notarize-check:
	@echo "Usage: make notarize-check-<uuid>"
notarize-check-%:
	xcrun altool \
	  --username "${NOTARIZE_APPLE_ID}" \
	  --password @env:NOTARIZE_APP_PASSWD \
	  --asc-provider "${NOTARIZE_ACCOUNT_ID}" \
	  --notarization-info "$*"


# Using the `osslsigncode` we can sign windows binaries from
# non-windows platforms.
codesign-windows: codesign-windows-launcher.exe  codesign-windows-osquery-extension.exe
codesign-windows-%: xp
	@if [ -z "${AUTHENTICODE_PASSPHRASE}" ]; then echo "Missing AUTHENTICODE_PASSPHRASE"; exit 1; fi
	osslsigncode -in build/windows/$*  -out build/windows/$*  -i https://kolide.com -h sha1 -t http://timestamp.verisign.com/scripts/timstamp.dll -pkcs12 ~/Documents/kolide-codesigning-2020.p12  -pass "${AUTHENTICODE_PASSPHRASE}"
	osslsigncode -in build/windows/$*  -out build/windows/$*  -i https://kolide.com -h sha256 -nest -ts http://sha256timestamp.ws.symantec.com/sha256/timestamp -pkcs12 ~/Documents/kolide-codesigning-2020.p12  -pass "${AUTHENTICODE_PASSPHRASE}"

codesign: notarize-darwin codesign-windows

package-builder: .pre-build deps
	go run cmd/make/make.go -targets=package-builder -linkstamp

package-builder-windows: .pre-build deps
	go run cmd/make/make.go -targets=package-builder -linkstamp --os windows
launcher-pummel:
	go run cmd/make/make.go -targets=launcher-pummel

deps-go:
	go run cmd/make/make.go -targets=deps-go,install-tools

deps: deps-go generate

.PHONY: generate
generate:
	go generate ./pkg/packagekit ./pkg/packaging
	go run cmd/make/make.go -targets=generate-tuf

.PHONY: proto
proto:
	@(cd pkg/pb/launcher; go generate)
	@(cd pkg/pb/querytarget; go generate)
	@echo "Generated code from proto definitions."

test: generate
	go test -cover -race $(shell go list ./... | grep -v /vendor/)

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
