#!/bin/sh

. /builder/prepare_workspace.inc
prepare_workspace || exit
make deps
make xp
