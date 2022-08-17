#!/bin/bash

set -e

mkdir -p ./debug/bin
yes | cp $(which osqueryd) ./debug/bin/osqueryd
make deps
make build
