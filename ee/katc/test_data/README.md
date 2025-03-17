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
can be found at `/Users/<my-username>/Library/Application Support/Google/Chrome/<Profile Name>/IndexedDB/file__0.indexeddb.leveldb`.
(The name of the indexeddb file will likely be the same, but you can confirm the origin
matches in Dev Tools in your browser by going to Application => IndexedDB => launchertestdb.)
On macOS, Firefox sqlite files can be found at a path similar to this one:
`/Users/<your-username>/Library/Application Support/Firefox/Profiles/*.default*/storage/default/file++++*+launcher+ee+katc+test_data+index.html/idb/*.sqlite`.

Zip the .indexeddb.leveldb directory (for Chrome) or the .sqlite file (for Firefox),
then move the zipped file to [indexeddbs](./indexeddbs). You can then reference this file
in the indexeddb tests.

## Troubleshooting parsing errors

First, it is helpful to understand how deserialization works. The general process is documented below,
but it is also helpful to read the deserialization code itself -- both in launcher, where we have
documented caveats and limitations as thoroughly as possible, and in the Chrome and Firefox code linked
in order to compare launcher's deserialization with the actual serialization and deserialization steps.

### General object deserialization process

The exact deserialization process differs a bit between the two browsers, but in general, we read byte-by-byte:

1. First, we will parse the header if present, continuing until we get a byte indicating the start of an object
2. We will read the next byte as a "token"/"tag" that indicates an upcoming string, which is the next object property name (e.g. "uuid")
3. We will read the next byte as the length of the upcoming string
4. We will read the n bytes as the string (e.g. "uuid")
5. We will read the next byte as a "token"/"tag", indicating the upcoming data type (for example, an int, an array, a nested object)
6. Depending on the upcoming data type, we may read the next byte as the number of expected bytes holding this data
7. We process the upcoming data according to its type; we read until either we hit an end token or have reached the number of expected bytes
8. We may have to read and discard metadata or padding at the end of the value
9. We set the object property name equal to its value, and continue
10. We repeat until we reach a token indicating we've reached the end of the object

For Chrome, review the code [here](https://github.com/v8/v8/blob/master/src/objects/value-serializer.cc).
For Firefox, review the code [here](https://searchfox.org/mozilla-central/source/js/src/vm/StructuredClone.cpp).

### Most likely parsing issues

In initial rollout and testing, we have come across parsing errors that mostly fall into two categories:

1. Data type not yet implemented
2. Not correctly discarding padding or metadata

If you are seeing the first error, the error message should hopefully make that clear. For example, the
error message might say "unsupported tag type" or "unimplemented array item type". In this case,
fixing the error entails reading the Chrome or Firefox source code and implementing deserialization for
the new data type. Note that if you have to implement deserialization of a new data type for one browser,
you will probably want to do it for the other browser too.

The second error is more difficult to figure out. The symptoms often look like misaligned data, where an
object property name might look like `id"1"name"jane"` -- i.e., several keys/values concatenated together
because the deserialization process hasn't correctly read the string length. In this case, the most useful
troubleshooting step is to determine the object property that was processed immediately prior to the malformed
data or the error, and then compare how that property is deserialized in launcher versus in the Chrome or
Firefox source code.

If you have access to the database exhibiting this issue, you can add it to the [indexeddbs](./indexeddbs/)
directory and run the tests in [table_test.go](../table_test.go) against this database. (However, unless
you handcrafted this database, you should not commit it to launcher, in case it contains sensitive data.)
Otherwise, for new data types, you can add the new data type to [main.js](./main.js) and update the existing
databases in order to test your fixes.

## References

* [Helpful tutorial for working with the IndexedDB API](https://developer.mozilla.org/en-US/docs/Web/API/IndexedDB_API/Using_IndexedDB)
* [Chrome serialization code](https://github.com/v8/v8/blob/master/src/objects/value-serializer.cc)
* [Firefox serialization code](https://searchfox.org/mozilla-central/source/js/src/vm/StructuredClone.cpp)
