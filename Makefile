all: build

.PHONY: build
build: launcher extension

.pre-build:
	mkdir -p build

extension: .pre-build
	go build -i -o build/osquery-extension.ext ./cmd/osquery-extension/

launcher: .pre-build
	go build -i -o build/launcher ./cmd/launcher/

deps:
	go get -u github.com/golang/dep/cmd/dep
	dep ensure -v

test:
	go test -race -cover -v "$(go list ./... | grep -v /vendor/)"

mac-pkg-builder:
	go build -i -o build/mac-pkg-builder ./cmd/mac-pkg-builder/

build-mac-pkg: launcher extension mac-pkg-builder
	mkdir -p bin/
	cp /usr/local/bin/osqueryd ./bin
	cp ./build/launcher ./bin
	cp ./build/osquery-extension.ext ./bin
	./build/mac-pkg-builder -key ${CLOUDREPO}/config/example_rsa.pem -package
	rm -rf bin
