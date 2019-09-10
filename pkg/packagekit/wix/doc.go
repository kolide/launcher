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

So Many Guids

Windows uses guids to identify how MSIs related to one another. Windows defines three codes:

  * UpgradeCode: Consistent for all releases of a product -- guid(name, identifier)
  * ProductCode: Identifies a product release -- guid(name, identifier, hostname, version)
  * PackageCode: Identifiers a MSI package itself -- guid(name, identifier, hostname, version)

For a given input, we need a stable guid. Convinently, md5 produces a
string of the correct length for these guids.

Note: MSIs can get quiet unhappy if they PackageCode changes, while
the ProductCode remains the same. That will result in an error about
"Another version of this product is already installed"

References

  1. http://wixtoolset.org/
  2. https://github.com/golang/build/blob/790500f5933191797a6638a27127be424f6ae2c2/cmd/release/releaselet.go#L224
  3. https://blogs.msdn.microsoft.com/pusu/2009/06/10/what-are-upgrade-product-and-package-codes-used-for/
  4. https://docs.microsoft.com/en-us/windows/desktop/Msi/preparing-an-application-for-future-major-upgrades
  5. https://docs.microsoft.com/en-us/windows/desktop/Msi/productcode

*/
package wix
