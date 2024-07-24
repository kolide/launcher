# Testing deserialization of IndexedDB data

Handcrafting data to test deserialization byte-by-byte is time-consuming and error-prone.
The JavaScript in this directory creates an IndexedDB database seeded with data
for use in tests.

## Instructions for generating new test IndexedDBs

Edit [main.js](./main.js) as desired. Open [index.html](./index.html) using Chrome or Firefox,
depending on the desired outcome.

Loading the page will populate IndexedDB with the desired data. You should also see a
console log message to this effect.

The file will now be available with other IndexedDB files. On macOS, Chrome indexeddb files
can be found at `/Users/<my-username>/Library/Application Support/Google/Chrome/Default/IndexedDB/file__0.indexeddb.leveldb`.
(The name of the indexeddb file will likely be the same, but you can confirm the origin
matches in Dev Tools in your browser by going to Application => IndexedDB => launchertestdb.)
On macOS, Firefox sqlite files can be found at a path similar to this one:
`/Users/<your-username>/Library/Application Support/Firefox/Profiles/*.default*/storage/default/file++++*+launcher+ee+katc+test_data+index.html/idb/*.sqlite`.

Zip the .indexeddb.leveldb directory (for Chrome) or the .sqlite file (for Firefox),
then move the zipped file to [indexeddbs](./indexeddbs). You can then reference this file
in the indexeddb tests.

If you are iteratively making changes to the database, Chrome will complain about re-creating
the database. You can delete the database via Dev Tools and then reload the page to re-create
the database successfully.

## References

* [Helpful tutorial for working with the IndexedDB API](https://developer.mozilla.org/en-US/docs/Web/API/IndexedDB_API/Using_IndexedDB)
