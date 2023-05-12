# Quick notes about mockery usage in this package

Mocks live in-package to avoid an import cycle (mocks must import this
package's `autoupdatableBinary` type; the package would have to import the mock
if it weren't in the package). The mock file names are suffixed with "_test" to
avoid shipping them.

Mocks can be generated or re-generated with the following command:

```
mockery --name <interface name> --filename=mock_<interface_name>_test.go --exported --inpackage
```

For example:

```
mockery --name librarian --filename mock_librarian_test.go --exported --inpackage
```
