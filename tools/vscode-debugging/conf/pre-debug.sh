#!/bin/bash

set -e

mkdir -p ./debug
make deps
make build
