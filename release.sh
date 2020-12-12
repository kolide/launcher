#!/bin/bash

# wraps the underlying make targets in some bash routines

set -e
set -o pipefail

# To build on the darwin platforms, we need to be on an m1 machine,
# and we need to have both arm and x86 go versions. As go release
# things, and our build/release stuff improves, this will probably
# change.
GOARM=/opt/homebrew/bin/go
GOX86=/Users/seph/go1.15.6.darwin-amd64/bin/go

rm -rf build

MAKE_GO=GOX86 make -j4 build_{launcher,"osquery-extension.ext"}_{darwin,windows,linux}_amd64
MAKE_GO=GOARM make -j4 build_{launcher,"osquery-extension.ext"}_{darwin}_arm64
