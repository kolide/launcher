#!/usr/bin/env node
"use strict";

/**
 * Creates a standard LevelDB using the default bytewise comparator (not idb_cmp1).
 * Run with Node.js: node create_bytewise_leveldb.js [outputDir]
 * Requires: npm install (in this directory)
 */

const path = require("path");
const { Level } = require("level");

const defaultOutDir = path.join(__dirname, "bytewise_leveldb");

async function main() {
  const outDir = process.argv[2] || defaultOutDir;

  const db = new Level(outDir, { valueEncoding: "utf8" });

  const entries = [
    ["test-stringvalue-key1", "test-stringvalue1"],
    ["test-intvalue-key2", 2],
    ["test-floatvalue-key3", 3.3],
    ["test-booleanvalue-key4", true],
  ];

  for (const [k, v] of entries) {
    await db.put(k, v);
  }

  await db.close();

  console.log("Successfully created bytewise LevelDB at:", outDir);
  console.log("Added", entries.length, "key/value pairs.");
  console.log("To use as a test fixture: zip this directory and place it in test_data/indexeddbs (e.g. bytewise.leveldb.zip).");
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
