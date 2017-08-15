#!/bin/bash

if [ ! -f "$1" ]; then
  echo "usage: $0 path/to/launcher.pkg"
  exit 1
fi

LAUNCHER_PKG=$1
TEMP_DIR=$(mktemp -d)

pkgutil --expand $LAUNCHER_PKG $TEMP_DIR/out
mkdir $TEMP_DIR/package
tar xzf $TEMP_DIR/out/Payload -C $TEMP_DIR/package
find $TEMP_DIR/package
