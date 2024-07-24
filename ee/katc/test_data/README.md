# Testing deserialization of FireFox IndexedDB data

Handcrafting data to test deserialization byte-by-byte is time-consuming and error-prone.

The data in this directory was generated as described in the [indexeddb package](../../indexeddb/test_data/README.md).
`index.html` is instead opened in Firefox, and the resulting database can instead be found in Firefox storage.
For example, on macOS, the sqlite file can be found at a path similar to this one:
`/Users/<your-username>/Library/Application Support/Firefox/Profiles/*.default*/storage/default/file++++*+launcher+ee+indexeddb+test_data+index.html/idb/*.sqlite`.
This sqlite database is then zipped and added to the [indexeddbs](./indexeddbs) directory.
