all: build

.PHONY: build

ifndef ($(GOPATH))
	GOPATH = $(HOME)/go
endif

PATH := $(GOPATH)/bin:$(PATH)

export GO111MODULE=on

# If not windows, set the shell to bash explicitly
ifneq ($(OS), Windows_NT)
	ifeq ($(shell uname), Darwin)
		SHELL := /bin/bash
	endif
endif


all: build
build: build_launcher build_osquery-extension.ext

.pre-build: ${BUILD_DIR}

${BUILD_DIR}:
ifeq ($(OS), Windows_NT)
	powershell New-Item -Type Directory -Force -Path ${BUILD_DIR} | powershell Out-Null
else
	mkdir -p ${BUILD_DIR}
endif

##
## Build
##

build_%: TARGET =  $(word 2, $(subst _, ,$@))
build_%: OS = $(word 3, $(subst _, ,$@))
build_%: OSARG = $(if $(OS), --os $(OS))
build_%: ARCH = $(word 4, $(subst _, ,$@))
build_%: ARCHARG = $(if $(ARCH), --arch $(ARCH))
build_%: GOARG = $(if $(CROSSGOPATH), --go $(CROSSGOPATH))
build_%: GOBUILD = $(if $(CROSSGOPATH), $(CROSSGOPATH), go)
build_%: .pre-build
	$(GOBUILD) run cmd/make/make.go -targets=$(TARGET) -linkstamp $(OSARG) $(ARCHARG) $(GOARG)

fake_%: TARGET =  $(word 2, $(subst _, ,$@))
fake_%: OS = $(word 3, $(subst _, ,$@))
fake_%: OSARG = $(if $(OS), --os $(OS))
fake_%: ARCH = $(word 4, $(subst _, ,$@))
fake_%: ARCHARG = $(if $(ARCH), --arch $(ARCH))
fake_%: .pre-build
	go run cmd/make/make.go -targets=$(TARGET) -linkstamp -fakedata $(OSARG) $(ARCHARG)

# The lipo command will combine things into universal
# binaries. Because of the go path needs, there is little point in
# abstracting this further
lipo_%: build/darwin.amd64/% build/darwin.arm64/%
	@mkdir -p build/darwin.universal
	lipo -create $^ -output build/darwin.universal/$*

# pointers, mostly for legacy reasons
launcher: build_launcher
tables.ext: build_tables.ext
extension: build_osquery-extension.ext
grpc.ext: build_grpc.ext
fake-launcher: fake_launcher


##
## Cross Build targets
##

RELEASE_TARGETS=launcher osquery-extension.ext package-builder
MANUAL_CROSS_OSES=darwin windows linux
ARM64_OSES=darwin
AMD64_OSES=darwin windows linux

# xp is a helper for quick cross platform builds, and sanity checking
# for breakage. humans only
xp: $(foreach target, $(RELEASE_TARGETS), $(foreach os, $(MANUAL_CROSS_OSES), build_$(target)_$(os)))

# Actual release targets. Because of the m1 cgo cross stuff, this requires explicit go paths
rel-amd64: CROSSGOPATH = /opt/go1.16.2.darwin-amd64/bin/go
rel-amd64: $(foreach target, $(RELEASE_TARGETS), $(foreach os, $(AMD64_OSES), build_$(target)_$(os)_amd64))

rel-arm64: CROSSGOPATH = /opt/go1.16.2.darwin-arm64/bin/go
rel-arm64: $(foreach target, $(RELEASE_TARGETS), $(foreach os, $(ARM64_OSES), build_$(target)_$(os)_arm64))

rel-lipo: $(foreach target, $(RELEASE_TARGETS), lipo_$(target))

##
## Release Process Stuff
##

RELEASE_VERSION = $(shell git describe --tags --always --dirty)

release:
	@echo "Run 'make release-phase1' on the m1 machine"
	@echo "Run 'make release-phase2' on a codesign machine"

release-phase1:
	rm -rf build
	$(MAKE) rel-amd64 rel-arm64
	$(MAKE) rel-lipo
#	$(MAKE) codesign
#	$(MAKE) binary-bundles

release-phase2:
	rm -rf build
	rsync -av 10.42.19.215:~/checkouts/kolide/launcher/build ./
#	$(MAKE) rel-amd64 rel-arm64
#	$(MAKE) rel-lipo
	$(MAKE) codesign
	$(MAKE) binary-bundles


# release: binary-bundle containers-push

binary-bundles:
	rm -rf build/binary-bundles
	$(MAKE) $(foreach p, $(shell cd build && ls -d */ | tr -d /), build/binary-bundles/$(p))

build/binary-bundles/%:
	mkdir -p build/binary-bundles
	mv build/$* build/$*_$(RELEASE_VERSION)
	cd build && zip -r "binary-bundles/$*_$(RELEASE_VERSION)".zip $*_$(RELEASE_VERSION)


##
## Handy osqueryi command line
##

osqueryi-tables: build_tables.ext
	osqueryd -S --allow-unsafe --verbose --extension ./build/tables.ext
osqueryi-tables-windows: build_tables.ext
	osqueryd.exe -S --allow-unsafe --verbose --extension .\build\tables.exe
sudo-osqueryi-tables: build_tables.ext
	sudo osqueryd -S --allow-unsafe --verbose --extension ./build/tables.ext
launchas-osqueryi-tables: build_tables.ext
	sudo launchctl asuser 0 osqueryd -S --allow-unsafe --verbose --extension ./build/tables.ext


# `-o runtime` should be enough, however there was a catalina bug that
# required we add `library`. This was fixed in 10.15.4. (from
# macadmins slack)
codesign-darwin:
	codesign --force -s "${CODESIGN_IDENTITY}" -v --options runtime,library --timestamp ./build/darwin*/*

notarize-darwin: codesign-darwin
	rm -f build/notarization-upload.zip
	zip -r build/notarization-upload.zip ./build/darwin*
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
codesign-windows-%: P12 = ~/Documents/kolide-codesigning-2021-04.p12
codesign-windows-%:
	@if [ -z "${AUTHENTICODE_PASSPHRASE}" ]; then echo "Missing AUTHENTICODE_PASSPHRASE"; exit 1; fi
	mv build/windows.amd64/$* build/windows.amd64/$*.tmp
	osslsigncode sign -in build/windows.amd64/$*.tmp  -out build/windows.amd64/$*  -i https://kolide.com -h sha1 -t http://timestamp.digicert.com -pkcs12 $(P12)  -pass "${AUTHENTICODE_PASSPHRASE}"
	rm build/windows.amd64/$*.tmp
	mv build/windows.amd64/$* build/windows.amd64/$*.tmp
	osslsigncode sign -in build/windows.amd64/$*.tmp  -out build/windows.amd64/$*  -i https://kolide.com -h sha256 -nest -ts http://sha256timestamp.ws.symantec.com/sha256/timestamp -pkcs12 $(P12)  -pass "${AUTHENTICODE_PASSPHRASE}"
	rm build/windows.amd64/$*.tmp

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
generate: deps-go
	go generate ./pkg/packagekit/... ./pkg/packaging/... ./pkg/osquery/tables/... ./pkg/augeas/...
	go run cmd/make/make.go -targets=generate-tuf

.PHONY: proto
proto:
	@(cd pkg/pb/launcher; go generate)
	@(cd pkg/pb/querytarget; go generate)
	@echo "Generated code from proto definitions."

test: generate
	go test -cover -coverprofile=coverage.out -race $(shell go list ./... | grep -v /vendor/)

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
	  | grep -v assets.go$ \
	  | xargs misspell -error -f 'misspell: {{ .Filename }}:{{ .Line }}:{{ .Column }}:corrected {{ printf "%q" .Original }} to {{ printf "%q" .Corrected }}'

lint-go-vet:
	go vet ./cmd/... ./pkg/...

lint-go-nakedret: deps-go
	nakedret ./pkg/... ./cmd/...

# This is ugly. since go-fmt doesn't have a simple exit code, we use
# some make trickery to handle failing if there;s output.
lint-go-fmt: $(foreach c,$(shell gofmt -l ./pkg/ ./cmd/ | grep -vE 'assets.go|bindata.go'),fmt-fail/$(c))
lint-go-fmt: deps-go
fmt-fail/%:
	@echo fmt failure in: $*
	@false

##
## Docker Tooling
##

CONTAINER_OSES = ubuntu16 ubuntu18 centos6 centos7 distroless

.PHONY: containers
containers: $(foreach c,$(CONTAINER_OSES),docker-$(c) dockerfake-$(c))
containers-push: $(foreach c,$(CONTAINER_OSES),dockerpush-$(c) dockerfakepush-$(c))

build-docker:
	docker build -t launcher-build  .

build-dockerfake:
	docker build -t launcher-fakedata-build --build-arg FAKE=-fakedata .

dockerfake-%:  #build-dockerfake
	@echo '#### Starting to build target: $@'
	docker build -t gcr.io/kolide-public-containers/launcher-fakedata-$* --build-arg FAKE=-fakedata docker/$*

docker-%: #build-docker
	@echo '#### Starting to build target: $@'
	docker build -t gcr.io/kolide-public-containers/launcher-$*  docker/$*

dockerpush-%: docker-%
	@echo '#### Starting to push target: $@'
	docker push gcr.io/kolide-public-containers/launcher-$*

dockerfakepush-%: dockerfake-%
	@echo '#### Starting to push target: $@'
	docker push gcr.io/kolide-public-containers/launcher-fakedata-$*
