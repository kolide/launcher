/*
Package wix is a lightweight wrapper around the wix tooolset.

It is heavily inspired by golang's build system (2)

Background and Theory Of Operations

wix's toolchain is based around compiling xml files into
installers. Wix provides a variety of tools that can help simply
this. This package leverages them.

The basic steps of making a package:
  1. Start with a xml file (TODO: consider a struct?)
  2. Create a packageRoot directory structure
  3. Use `heat` to harvest the file list from the packageRoot
  4. Use `candle` to take the wxs from (1) and (3) and make a wixobj
  5. Use `light` to compile thje wixobj into an msi


While this is a somewhat agnostic wrapper, it does make several
assumptions about the underlying process. It is not meant as a
complete wix wrapper.


References

  1. http://wixtoolset.org/
  2. https://github.com/golang/build/blob/790500f5933191797a6638a27127be424f6ae2c2/cmd/release/releaselet.go#L224

*/
package wix
