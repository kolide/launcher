#!/bin/sh

# upstart's stop and restart commands error out if the daemon isn't
# running. So stop and start are separate, and `set -e` is after the
# stop.

stop launcher-{{.Identifier}}
set -e
start launcher-{{.Identifier}}
