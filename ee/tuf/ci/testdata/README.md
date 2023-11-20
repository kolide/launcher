Compressing test binaries during each test run takes too long, so we provide
pre-compressed binaries for the test runs to use here.

Each binary contained in the tarballs was generated on the appropriate platform
using the following main.go file:

```
package main

import (
	"fmt"
	"runtime"
)

func main() {
	fmt.Printf("Hello from %s.%s\n", runtime.GOOS, runtime.GOARCH)
}
```
