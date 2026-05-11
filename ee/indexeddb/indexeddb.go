// Package indexeddb provides the ability to query an IndexedDB and parse the objects it contains.
package indexeddb

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/golang/snappy"
	"github.com/kolide/goleveldb/leveldb"
	leveldbcomparer "github.com/kolide/goleveldb/leveldb/comparer"
	leveldberrors "github.com/kolide/goleveldb/leveldb/errors"
	"github.com/kolide/goleveldb/leveldb/opt"
	"github.com/kolide/launcher/v2/ee/agent"
	"github.com/kolide/launcher/v2/ee/observability"
	"github.com/kolide/launcher/v2/pkg/indexeddbcomparator"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

// maxNumberOfObjectStoresToCheck is the number of indices for object stores we will check
// before declaring failure to find the given object store. We cannot look up
// object stores by their names, only by their IDs -- so we have to iterate through
// up to maxNumberOfObjectStoresToCheck IDs to find the desired store. We assume there are
// fewer than 100 object stores in a given database. (We may need to adjust this
// number upward after further research, but for now this seems like a safe upper
// bounds.)
const maxNumberOfObjectStoresToCheck = 100

const (
	// Some headers may indicate that the payload is wrapped -- requiring snappy decompression
	// or blob substition. See: https://chromium.googlesource.com/chromium/src/+/main/third_party/blink/renderer/modules/indexeddb/idb_value_wrapping.cc
	tokenRequiresProcessingSSVPseudoVersion byte = 0x11
	tokenReplaceWithBlob                    byte = 0x01
	tokenCompressedWithSnappy               byte = 0x02

	externalObjectTypeBlob                   = 0x00 // 0
	externalObjectTypeFile                   = 0x01 // 1
	externalObjectTypeFileSystemAccessHandle = 0x02 // 2
)

// QueryIndexeddbObjectStore queries the indexeddb at the given location `dbLocation`,
// returning all objects in the given database that live in the given object store.
func QueryIndexeddbObjectStore(ctx context.Context, slogger *slog.Logger, dbLocation string, dbName string, objectStoreName string, comparer string) ([]map[string][]byte, error) {
	ctx, span := observability.StartSpan(ctx, "db_name", dbName, "object_store_name", objectStoreName)
	defer span.End()

	// If Chrome is open, we won't be able to open the db. So, copy it to a temporary location first.
	tempDbCopyLocation, err := CopyLeveldb(ctx, dbLocation)
	if err != nil {
		if tempDbCopyLocation != "" {
			_ = os.RemoveAll(tempDbCopyLocation)
		}
		return nil, fmt.Errorf("unable to copy db: %w", err)
	}
	// The copy was successful -- make sure we clean it up after we're done
	defer os.RemoveAll(tempDbCopyLocation)

	objs := make([]map[string][]byte, 0)

	db, err := OpenLeveldb(ctx, slogger, tempDbCopyLocation, comparer)
	if err != nil {
		return nil, fmt.Errorf("opening leveldb: %w", err)
	}
	defer db.Close()
	span.AddEvent("db_opened")

	// Get the database ID from the name
	databaseNameKey, err := databaseIdKey(dbLocation, dbName)
	if err != nil {
		return nil, fmt.Errorf("getting database id key: %w", err)
	}
	databaseIdRaw, err := db.Get(databaseNameKey, nil)
	if err != nil {
		// If the database doesn't exist, return an empty list of objects
		if errors.Is(err, leveldberrors.ErrNotFound) {
			return objs, nil
		}
		return nil, fmt.Errorf("querying for database id: %w", err)
	}
	databaseId, _ := binary.Uvarint(databaseIdRaw)

	// We can't query for the object store ID by its name -- we have to query each ID to get its name,
	// and check against that. Object store indices start at 1.
	var objectStoreId uint64
	var i uint64
	for i = 1; i <= maxNumberOfObjectStoresToCheck; i++ {
		objectStoreNameRaw, err := db.Get(objectStoreNameKey(databaseId, i), nil)
		if err != nil {
			continue
		}
		foundObjectStoreName, err := decodeUtf16BigEndianBytes(objectStoreNameRaw)
		if err != nil {
			continue
		}
		if string(foundObjectStoreName) == objectStoreName {
			objectStoreId = i
			break
		}
	}
	span.AddEvent("got_object_store_id")

	if objectStoreId == 0 {
		// If the object store doesn't exist, return an empty list of objects
		return objs, nil
	}

	// Get the key prefix for all objects in this store.
	keyPrefix := objectDataKeyPrefix(databaseId, objectStoreId)
	blobPrefix := blobKeyPrefix(databaseId, objectStoreId)

	// Now, we can read all records, keeping only the ones with our matching key prefix.
	iter := db.NewIterator(nil, nil)
	for iter.Next() {
		key := iter.Key()

		if !bytes.HasPrefix(key, keyPrefix) {
			continue
		}

		tmp := make([]byte, len(iter.Value()))
		copy(tmp, iter.Value())

		// Check for external object data associated with this key, in case this value is wrapped
		var externalObjectFilenames []string
		userKey := bytes.TrimPrefix(key, keyPrefix)
		blobKey := append(blobPrefix, userKey...)
		externalObjectsRaw, err := db.Get(blobKey, nil)
		if err != nil {
			// We expect ErrNotFound when there is no external object data associated with this key
			if !errors.Is(err, leveldb.ErrNotFound) {
				slogger.Log(ctx, slog.LevelWarn,
					"reading blob data from db",
					"err", err,
				)
			}
		} else {
			// External objects data exists for this key -- parse it
			externalObjectFilenames, err = readExternalObjects(externalObjectsRaw, databaseId, tempDbCopyLocation)
			if err != nil {
				slogger.Log(ctx, slog.LevelWarn,
					"parsing external objects data from db",
					"err", err,
				)
			}
		}

		// handleWrappedValues returns row unmodified if it is not a wrapped value
		if unwrapped, err := handleWrappedValues(tmp, externalObjectFilenames); err != nil {
			slogger.Log(ctx, slog.LevelError,
				"could not unwrap wrapped value -- skipping row",
				"err", err,
			)
		} else {
			objs = append(objs, map[string][]byte{
				"data": unwrapped,
			})
		}
	}
	iter.Release()
	if err := iter.Error(); err != nil {
		return objs, fmt.Errorf("iterator error: %w", err)
	}
	span.AddEvent("completed_iteration")

	return objs, nil
}

// readExternalObjects reads through the list of external objects (blobs, files, and file handles)
// contained in `value`.
// See https://github.com/chromium/chromium/blob/main/content/browser/indexed_db/docs/leveldb_coding_scheme.md#externalobject-value
func readExternalObjects(value []byte, databaseId uint64, blobRootDir string) ([]string, error) {
	valueReader := bytes.NewReader(value)
	filepaths := make([]string, 0)

	// There is no length for these objects -- read until we've read everything in `value`.
	for {
		// First up is the object type
		objectType, err := valueReader.ReadByte()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("reading external object type: %w", err)
		}

		// For blobs and files, the next fields are blob_number (varint), type (StringWithLength),
		// and size (varint)
		if objectType == externalObjectTypeBlob || objectType == externalObjectTypeFile {
			// blob_number
			blobNumber, err := binary.ReadUvarint(valueReader)
			if err != nil {
				return nil, fmt.Errorf("reading blob_number: %w", err)
			}

			// With the blob number, we are now able to extract the filepath for blob types
			filepaths = append(filepaths, filepathForBlob(blobNumber, databaseId, blobRootDir))

			// type
			if _, err := readStringWithLength(valueReader); err != nil {
				return nil, fmt.Errorf("reading type: %w", err)
			}

			// size
			if _, err := binary.ReadUvarint(valueReader); err != nil {
				return nil, fmt.Errorf("reading size: %w", err)
			}
		}

		// For files, the next field is filename (StringWithLength) and lastModified (varint)
		if objectType == externalObjectTypeFile {
			// filename
			if _, err := readStringWithLength(valueReader); err != nil {
				return nil, fmt.Errorf("reading filename: %w", err)
			}

			// lastModified
			if _, err := binary.ReadUvarint(valueReader); err != nil {
				return nil, fmt.Errorf("reading lastModified: %w", err)
			}
		}

		// For file system access handles, the next field is token (BinaryWithLength).
		if objectType == externalObjectTypeFileSystemAccessHandle {
			// Binary - VarInt prefix with length in bytes, followed by data bytes
			binaryDataLength, err := binary.ReadUvarint(valueReader)
			if err != nil {
				return nil, fmt.Errorf("reading binary data length: %w", err)
			}

			if binaryDataLength > uint64(valueReader.Len()) {
				return nil, fmt.Errorf("cannot read BinaryWithLength: length %d but only %d bytes remaining to read", binaryDataLength, valueReader.Len())
			}

			// Read and discard data -- we don't do anything with it currently.
			for i := 0; i < int(binaryDataLength); i++ {
				if _, err := valueReader.ReadByte(); err != nil {
					return nil, fmt.Errorf("reading byte at index %d in binary data of length %d: %w", i, binaryDataLength, err)
				}
			}

			// We don't handle this type currently, but we still need an entry in the `filepaths` list
			// so that indexing works correctly.
			filepaths = append(filepaths, "")
		}
	}

	return filepaths, nil
}

func filepathForBlob(blobNumber uint64, databaseId uint64, blobRootDir string) string {
	blobDir := fmt.Sprintf("%02x", (blobNumber&0xff00)>>8)
	blobFilename := fmt.Sprintf("%x", blobNumber)
	blobFilepath := filepath.Join(blobRootDir, fmt.Sprintf("%d", databaseId), blobDir, blobFilename)
	return blobFilepath
}

// readStringWithLength reads the upcoming StringWithLength from the byte reader.
// It takes the following format:
// VarInt prefix with length in code units (i.e. bytes/2), followed by String (UTF-16 BE)
func readStringWithLength(valueReader *bytes.Reader) (string, error) {
	stringLengthInCodeUnits, err := binary.ReadUvarint(valueReader)
	if err != nil {
		return "", fmt.Errorf("reading string length: %w", err)
	}

	stringLength := stringLengthInCodeUnits * 2
	if stringLength > uint64(valueReader.Len()) {
		return "", fmt.Errorf("cannot read StringWithLength: length %d but only %d bytes remaining to read", stringLength, valueReader.Len())
	}

	rawString := make([]byte, stringLength)
	for i := 0; i < int(stringLength); i++ {
		nextByte, err := valueReader.ReadByte()
		if err != nil {
			return "", fmt.Errorf("reading byte at index %d in string of length %d: %w", i, stringLength, err)
		}
		rawString[i] = nextByte
	}

	// Strings are UTF-16 BE
	decoded, _, err := transform.Bytes(unicode.UTF16(unicode.BigEndian, unicode.IgnoreBOM).NewDecoder(), rawString)
	if err != nil {
		return "", fmt.Errorf("reading string as utf-16: %w", err)
	}

	return string(decoded), nil
}

// handleWrappedValues examines `payload` to see if it is a wrapped value --
// either a blob that must be replaced, or snappy-compressed data that must
// be decompressed.
func handleWrappedValues(payload []byte, blobFilepathList []string) ([]byte, error) {
	// First, read the indexeddb version, which precedes the serialized value.
	// See: https://github.com/chromium/chromium/blob/master/content/browser/indexed_db/docs/leveldb_coding_scheme.md#object-store-data
	_, bytesRead := binary.Uvarint(payload)
	if bytesRead == 0 {
		return payload, nil
	}
	header := payload[:bytesRead]

	// Wrapped values are determined by the presence of the following sequence in the header payload:
	// 1) 0xFF - kVersionTag
	// 2) 0x11 - kRequiresProcessingSSVPseudoVersion
	// 3) 0x01 or 0x02 - the wrap type -- kReplaceWithBlob or kCompressedWithSnappy
	if len(payload) >= 4+bytesRead &&
		payload[bytesRead] == tokenVersion &&
		payload[bytesRead+1] == tokenRequiresProcessingSSVPseudoVersion {
		var unwrapped []byte
		var err error

		switch payload[bytesRead+2] {
		case tokenCompressedWithSnappy:
			unwrapped, err = snappyDecompressedIfNeeded(payload[bytesRead+3:])
		case tokenReplaceWithBlob:
			unwrapped, err = replaceWithBlobIfNeeded(payload[bytesRead+3:], blobFilepathList)
		default:
			return payload, nil
		}
		if err != nil {
			return nil, fmt.Errorf("unwrapping value: %w", err)
		}
		return append(header, unwrapped...), nil
	}

	return payload, nil
}

// snappyDecompressedIfNeeded decompresses the payload if it is compressed with Snappy.
func snappyDecompressedIfNeeded(payload []byte) ([]byte, error) {
	decompressed, err := snappy.Decode(nil, payload)
	if err != nil {
		return nil, fmt.Errorf("snappy decompress after Chrome FF/11/02 wrapper: %w", err)
	}

	if len(decompressed) == 0 {
		return nil, errors.New("snappy decompression yielded empty data set")
	}

	return decompressed, nil
}

func replaceWithBlobIfNeeded(payload []byte, blobFilepathList []string) ([]byte, error) {
	payloadReader := bytes.NewReader(payload)

	// Next up in the header is the blob size.
	if _, err := binary.ReadUvarint(payloadReader); err != nil {
		return nil, fmt.Errorf("reading blob size: %w", err)
	}

	// Next comes the blob offset, the offset of the SSV-wrapping Blob in the IDBValue list of Blobs.
	blobOffset, err := binary.ReadUvarint(payloadReader)
	if err != nil {
		return nil, fmt.Errorf("reading blob offset: %w", err)
	}

	if blobOffset >= uint64(len(blobFilepathList)) {
		return nil, fmt.Errorf("wanted blob with offset %d, but only have %d blobs", blobOffset, len(blobFilepathList))
	}

	blobFile := blobFilepathList[blobOffset]
	if len(blobFile) == 0 {
		return nil, fmt.Errorf("no blob file available at offset %d (may be file access handle)", blobOffset)
	}

	blobBytes, err := os.ReadFile(blobFilepathList[blobOffset])
	if err != nil {
		return nil, fmt.Errorf("reading blob file %s: %w", blobFilepathList[blobOffset], err)
	}

	return blobBytes, nil
}

func CopyLeveldb(ctx context.Context, sourceDb string) (string, error) {
	ctx, span := observability.StartSpan(ctx)
	defer span.End()

	dbCopyLocation, err := agent.MkdirTemp(filepath.Base(sourceDb))
	if err != nil {
		return "", fmt.Errorf("making temporary directory: %w", err)
	}

	entries, err := os.ReadDir(sourceDb)
	if err != nil {
		return dbCopyLocation, fmt.Errorf("reading directory contents: %w", err)
	}
	for _, entry := range entries {
		// We expect only files in the database -- no directories, symlinks, etc.
		// Ignore any unexpected files.
		if entry.IsDir() || !entry.Type().IsRegular() {
			continue
		}
		// We don't want to copy over the lock -- we won't be able to open it for reading.
		if entry.Name() == "LOCK" {
			continue
		}
		src := filepath.Join(sourceDb, entry.Name())
		dest := filepath.Join(dbCopyLocation, entry.Name())
		if err := copyFile(ctx, src, dest); err != nil {
			return dbCopyLocation, fmt.Errorf("copying file: %w", err)
		}
	}

	// We want to copy over both the leveldb files (in sourceDb, some/path/to/indexeddb.leveldb)
	// and the blob files (in /some/path/to/indexeddb.blob) -- so let's look for the blob files
	// now too.
	blobDir := strings.TrimSuffix(sourceDb, ".leveldb") + ".blob"
	if _, err := os.Stat(blobDir); err != nil {
		// Either the blob directory doesn't exist, or we can't access it -- proceed without.
		return dbCopyLocation, nil
	}
	if err := filepath.WalkDir(blobDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("error walking blob directory: %w", err)
		}

		dest := filepath.Join(dbCopyLocation, strings.TrimPrefix(path, blobDir))
		if d.IsDir() {
			if err := os.MkdirAll(dest, 0755); err != nil {
				return fmt.Errorf("making directory %s: %w", dest, err)
			}
			return nil
		}

		if err := copyFile(ctx, path, dest); err != nil {
			return fmt.Errorf("copying file %s: %w", path, err)
		}

		return nil
	}); err != nil {
		return dbCopyLocation, fmt.Errorf("copying blob files: %w", err)
	}

	return dbCopyLocation, nil
}

func copyFile(ctx context.Context, src string, dest string) error {
	_, span := observability.StartSpan(ctx)
	defer span.End()

	srcFh, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("opening %s: %w", src, err)
	}
	defer srcFh.Close()

	destFh, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("opening %s: %w", dest, err)
	}

	if _, err := io.Copy(destFh, srcFh); err != nil {
		_ = destFh.Close()
		return fmt.Errorf("copying %s to %s: %w", src, dest, err)
	}

	if err := destFh.Close(); err != nil {
		return fmt.Errorf("completing write from %s to %s: %w", src, dest, err)
	}

	return nil
}

func OpenLeveldb(ctx context.Context, slogger *slog.Logger, dbLocation string, comparer string) (*leveldb.DB, error) {
	_, span := observability.StartSpan(ctx)
	defer span.End()

	opts := &opt.Options{
		Comparer:               comparerFromType(comparer, slogger),
		DisableSeeksCompaction: true,               // no need to perform compaction
		Strict:                 opt.StrictRecovery, // we prefer to drop corrupted data rather than fail to open the db altogether
	}

	// we've seen a failure when querying some leveldbs with the historical bytewise comparer like:
	// opening leveldb: opening db: `leveldb/table: Writer: keys are not in increasing order
	// this is expected and not an issue for historical bytewise, so we force the db into read-only mode to avoid this
	if comparer == "historical_bytewise" {
		opts.ReadOnly = true
	}

	db, dbOpenErr := leveldb.OpenFile(dbLocation, opts)
	if dbOpenErr != nil {
		// TODO we should update goleveldb to return a specific, checkable error type for this case so we don't have to do this gross string check
		// error looks like- leveldb: manifest corrupted (field 'comparer'): mismatch: want 'idb_cmp1', got 'leveldb.BytewiseComparator'
		if strings.Contains(dbOpenErr.Error(), "mismatch: want 'idb_cmp1', got 'leveldb.BytewiseComparator'") {
			// try again with the default comparer
			opts.Comparer = leveldbcomparer.DefaultComparer
			db, dbOpenErr = leveldb.OpenFile(dbLocation, opts)
			// if this fixed the issue, return the db. otherwise continue on to try recovery,
			// we know that we're better off in this scenario with the bytewise comparator anyway
			if dbOpenErr == nil {
				return db, nil
			}
		}
		// ensure we log this error so we can investigate. we don't think we're seeing any non-idb_cmp1
		// leveldbs, but when that is the case we still get a valid db returned, and then no errors from
		// the RecoverFile call, so it is possible that this is a valid corruption error which recovery wouldn't have actually fixed.
		// we can track this error and see if there are other comparer types in the wild that should be accounted for
		slogger.Log(context.TODO(), slog.LevelError,
			"error opening leveldb, will attempt recovery",
			"err", dbOpenErr.Error(),
		)

		// Perform recover in case we missed something while copying
		var dbRecoverErr error
		db, dbRecoverErr = leveldb.RecoverFile(dbLocation, opts)
		if dbRecoverErr != nil {
			return nil, fmt.Errorf("opening db: `%+v`; recovering db: %w", dbOpenErr, dbRecoverErr)
		}
	}

	return db, nil
}

// comparerFromType returns the appropriate comparer for the given comparer type.
// if unset or invalid, it returns our default comparer, idb_cmp1.
func comparerFromType(comparerType string, slogger *slog.Logger) leveldbcomparer.Comparer {
	switch comparerType {
	case "historical_bytewise":
		return HistoricalBytewiseComparer()
	case "default_bytewise":
		return leveldbcomparer.DefaultComparer
	case "", "idb_cmp1":
		return indexeddbcomparator.NewIdbCmp1Comparer(slogger)
	default:
		return indexeddbcomparator.NewIdbCmp1Comparer(slogger)
	}
}
