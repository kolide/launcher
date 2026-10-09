// Stub osqueryd binary which prints out a compiled version
// in the format the real binary provides.
package main

import "fmt"

var version string

func main() {
	fmt.Printf("osqueryd version %s\n", version)
}
