#!/bin/sh

set -e

systemctl daemon-reload

systemctl enable launcher.{{.Identifier}}
systemctl restart launcher.{{.Identifier}}
