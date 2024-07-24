# Testing deserialization of IndexedDB data

Handcrafting data to test deserialization byte-by-byte is time-consuming and error-prone.
The JavaScript in this directory creates an IndexedDB database seeded with data
for use in tests.

## Instructions for generating new test indexeddb.leveldb files

Edit [main.js](./main.js) as desired. Open [index.html](./index.html) using Google Chrome.

Loading the page will populate IndexedDB with the desired data. You should also see a
console log message to this effect.

The file will now be available with other Chrome IndexedDB files. For example, on macOS,
I located the resulting file at `/Users/<my-username>/Library/Application Support/Google/Chrome/Default/IndexedDB/file__0.indexeddb.leveldb`.
The name of the indexeddb file will likely be the same, but you can confirm the origin
matches in Dev Tools in your browser by going to Application => IndexedDB => launchertestdb.

Zip the directory, then move the zipped file to [indexeddbs](./indexeddbs). You can then
reference this file in the indexeddb tests.

If you are iteratively making changes to the database, Chrome will complain about re-creating
the database. You can delete the database via Dev Tools and then reload the page to re-create
the database successfully.

## References

* [Helpful tutorial for working with the IndexedDB API](https://developer.mozilla.org/en-US/docs/Web/API/IndexedDB_API/Using_IndexedDB)
