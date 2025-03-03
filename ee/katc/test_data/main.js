(function () {
    const databaseName = "launchertestdb";
    const objectStoreName = "launchertestobjstore";
    const objectStoreKeyPath = "uuid";

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
                email: "test1@example.com",
                someTimestamp: 1720034607, // *unint32
                someDate: new Date(), // Date object, empty
                someMap: new Map(), // Map object, empty
                someSet: new Set() // Set object, empty
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
                email: "test2@example.com",
                someTimestamp: 1726096312, // *unint32
                someDate: new Date("December 17, 1995 03:24:00"), // Date object, not empty
                someMap: new Map([
                    [1, "one"],
                    [2, "two"],
                    [3, "three"],
                ]), // Map object, not empty
                someSet: new Set(["a", "b", "c"]) // Set object
            },
        ];
        objectStore.transaction.oncomplete = (event) => {
            // Store values in the newly created objectStore.
            const objectStoreTransaction = db
                .transaction(objectStoreName, "readwrite")
                .objectStore(objectStoreName);
            storeData.forEach((row) => {
                objectStoreTransaction.add(row);
            });
            console.log("Added all data to IndexedDB")
        };
    }
})();
