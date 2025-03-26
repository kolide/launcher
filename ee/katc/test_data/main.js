(function () {
    const databaseName = "launchertestdb";
    const objectStoreName = "launchertestobjstore";
    const objectStoreKeyPath = "uuid";

    // In case the database already exists -- delete it so we can refresh the data.
    const deleteReq = window.indexedDB.deleteDatabase(databaseName);
    deleteReq.onerror = (event) => {
        console.log(event.error);
    }
    deleteReq.onsuccess = (event) => {
        console.log("Deleted db in preparation for re-creation");
    }

    // Open database. Second arg is database version. We're not dealing with
    // migrations at the moment, so just leave it at 1.
    const request = window.indexedDB.open(databaseName, 1);
    request.onupgradeneeded = (event) => {
        // Create an object store. We're going to skip creating indices for now
        // because this package doesn't make use of them.
        const db = event.target.result;
        const objectStore = db.createObjectStore(objectStoreName, { keyPath: objectStoreKeyPath });

        // Create a sparse array
        let sparseArr = ["zero", "one"];
        // a[2] and a[3] skipped, making this array sparse
        sparseArr[4] = "four";

        // Create a sparse array with a different data type in it
        let secondSparseArray = [1, 1];
        secondSparseArray[6] = 1;

        // Create some TypedArrays with a variety of types, some with explicit underlying ArrayBuffers
        const unsignedIntArrBuf = new ArrayBuffer(1);
        const unsignedIntArr = new Uint8Array(unsignedIntArrBuf);
        unsignedIntArr[0] = 20;
        const signedIntArrBuf = new ArrayBuffer(16);
        const signedIntArr = new Int32Array(signedIntArrBuf);
        signedIntArr[2] = 3000;
        const floatArr = new Float64Array([4.4, 4.5]);
        const clampedUintArr = new Uint8ClampedArray([1000, -20]); // clamped to [255, 0]
        const withOffsetArrBuf = new ArrayBuffer(32);
        const withOffsetArr = new Uint16Array(withOffsetArrBuf, 2);
        withOffsetArr[4] = 101;
        withOffsetArr[6] = 201;
        const lengthTrackingArrBuf = new ArrayBuffer(8, { maxByteLength: 16 });
        const lengthTrackingArr = new Float32Array(lengthTrackingArrBuf);
        lengthTrackingArrBuf.resize(12); // will also resize lengthTrackingArr

        // Shove some data in the object store. We want at least a couple items,
        // and a variety of data structures to really test our deserialization.
        const storeData = [
            {
                uuid: "0b438872-8b65-4e99-9cd4-95f0eeac2ad6", // ASCII string
                name: String.fromCodePoint(0x1F920), // UTF-16 string -- this is a 🤠 cowboy hat face emoji
                version: 35, // Integer: int32
                preferences: null, // Null
                flags: // Nested object
                    {
                        someFeatureEnabled: true, // Boolean: true
                        betaEnabled: false, // Boolean: false
                    },
                aliases: // Dense array of strings
                    [
                        "alias1",
                        "alias2"
                    ],
                linkedIds: sparseArr, // Sparse array of strings
                anotherSparseArray: secondSparseArray, // Sparse array of integers
                someDetails: // Dense array of nested objects
                    [
                        {
                            "id": 1,
                            "name": "detail1",
                            "enabled": true
                        },
                        {
                            "id": 2,
                            "name": "detail2",
                            "enabled": false
                        },
                        {
                            "id": 3,
                            "name": "detail3",
                            "enabled": true
                        }
                    ],
                noDetails: [], // Empty array
                numArray: [1, 2, 3], // Dense array of numbers
                email: "test1@example.com",
                someTimestamp: 1720034607, // *uint32
                someDate: new Date(), // Date object, empty
                someMap: new Map(), // Map object, empty
                someComplexMap: new Map([
                    ["set_a", new Set(["set_a_item1", "set_a_item2", "set_a_item3"])],
                    [new Set(["b1", "b2"]), "set_b"],
                ]), // Map object with complex items
                someSet: new Set(), // Set object, empty
                someRegex: new RegExp("\\w+", "sm"), // Regex
                someStringObject: new String(""), // String object, empty
                someNumberObject: new Number(0), // Number object, empty
                someDouble: 0.0, // double
                someBoolean: new Boolean(true), // Boolean object, true
                someTypedArray: unsignedIntArr, // TypedArray, Uint8Array (covers unsigned int types)
                someArrayBuffer: unsignedIntArr.buffer, // ArrayBuffer object
                anotherTypedArray: floatArr, // TypedArray, Float64Array (covers float types)
                yetAnotherTypedArray: withOffsetArr, // TypedArray, Uint16Array with byte offset
            },
            {
                uuid: "03b3e669-3e7a-482c-83b2-8a800b9f804f",
                name: String.fromCodePoint(0x1F354), // UTF-16 string -- this is a 🍔 burger emoji
                version: 100000, // Integer: int32
                preferences: null, // Null
                flags: // Nested object
                    {
                        someFeatureEnabled: false, // Boolean: false
                        betaEnabled: true, // Boolean: true
                    },
                aliases: // Dense array of strings
                    [
                        "anotheralias1"
                    ],
                linkedIds: sparseArr, // Sparse array of strings
                anotherSparseArray: secondSparseArray, // Sparse array of integers
                someDetails: // Dense array of nested objects
                    [
                        {
                            "id": 1,
                            "name": "detail1",
                            "enabled": false
                        },
                        {
                            "id": 2,
                            "name": "detail2",
                            "enabled": true
                        }
                    ],
                noDetails: [], // Empty array
                numArray: [1, 2, 3], // Dense array of numbers
                email: "test2@example.com",
                someTimestamp: 1726096312, // *uint32
                someDate: new Date("December 17, 1995 03:24:00"), // Date object, not empty
                someMap: new Map([
                    [1, "one"],
                    [2, "two"],
                    [3, "three"],
                ]), // Map object, not empty
                someComplexMap: new Map([
                    [1726096500, [{"id": 2}]],
                    [new Map([["someNestedMapKey", false]]), null],
                ]), // Map object with complex items
                someSet: new Set(["a", "b", "c"]), // Set object
                someRegex: new RegExp("[abc]", "i"), // Regex
                someStringObject: new String("testing"), // String object
                someNumberObject: new Number(123456.789), // Number object
                someDouble: 304.302, // double
                someBoolean: new Boolean(false), // Boolean object, false
                someTypedArray: signedIntArr, // TypedArray, Int32Array (covers signed int types)
                someArrayBuffer: signedIntArr.buffer, // ArrayBuffer object
                anotherTypedArray: clampedUintArr, // TypedArray, Uint8ClampedArray (covers clamped types)
                yetAnotherTypedArray: lengthTrackingArr, // TypedArray, Float32Array with length tracking
            },
        ];
        objectStore.transaction.oncomplete = (event) => {
            // Store values in the newly-created objectStore.
            const objectStoreTransaction = db
                .transaction(objectStoreName, "readwrite")
                .objectStore(objectStoreName);
            storeData.forEach((row) => {
                objectStoreTransaction.add(row);
            });
            objectStoreTransaction.onsuccess = (event) => {
                console.log("Added all data to IndexedDB");
            };
            objectStoreTransaction.onerror = (event) => {
                console.log("Error adding data to database", event.error);
            };
        };
        objectStore.transaction.onerror = (event) => {
            console.log("Error creating object store", event.error);
        };
    };

    request.onerror = (event) => {
        console.log("Error creating database", request.error);
    };
    request.onsuccess = (event) => {
        console.log("Successfully created database");
    };
})();
