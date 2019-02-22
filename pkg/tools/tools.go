// +build tools

/*
Package tools tracks imports of binary Go dependencies. This is done in order to version the utilities together with the checked in code.
For additional documentation on the topic:
	https://github.com/golang/go/wiki/Modules#how-can-i-track-tool-dependencies-for-a-module
	https://github.com/golang/go/issues/25922#issuecomment-412992431
*/
package tools

import (
	_ "github.com/alexkohler/nakedret"
	_ "github.com/client9/misspell/cmd/misspell"
	_ "github.com/go-bindata/go-bindata/go-bindata"
	_ "github.com/tsenart/deadcode"
)
