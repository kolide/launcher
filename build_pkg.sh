#!/bin/bash

launcherRepo=$GOPATH/src/github.com/kolide/launcher
tempDirectory=$(mktemp -d)
packagingRoot=$tempDirectory/pkgroot

mkdir -p $packagingRoot/etc/kolide
mkdir -p $packagingRoot/var/kolide
mkdir -p $packagingRoot/usr/local/kolide/bin

cp $HOME/Desktop/secret $packagingRoot/etc/kolide/secret
cp /usr/local/bin/osqueryd $packagingRoot/usr/local/kolide/bin/osqueryd
cp $launcherRepo/build/launcher $packagingRoot/usr/local/kolide/bin/launcher
cp $launcherRepo/build/osquery-extension.ext $packagingRoot/usr/local/kolide/bin/osquery-extension.ext
cp -R $launcherRepo/tools/packaging/macos/root/ $packagingRoot

tree $packagingRoot

pkgbuild \
  --root $packagingRoot \
  --scripts $launcherRepo/tools/packaging/macos/scripts \
  --identifier com.kolide.osquery \
  --version 1.0.0 \
  $HOME/Desktop/launcher.pkg

rm -rf $tempDirectory
