all: build

.PHONY: build
build: launcher extension

.pre-build:
	mkdir -p build

extension: .pre-build
	go build -o build/osquery-extension.ext ./cmd/osquery-extension/

launcher: .pre-build
	go build -o build/launcher ./cmd/launcher/

deps:
	go get -u github.com/golang/dep/cmd/dep
	dep ensure

test:
	go test -race -cover -v "$(go list ./... | grep -v /vendor/)"
