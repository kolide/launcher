package jwt

import (
	"context"
	"crypto"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"log/slog"
	"maps"
	"os"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/kolide/launcher/v2/ee/agent/types"
	"github.com/kolide/launcher/v2/ee/dataflatten"
	"github.com/kolide/launcher/v2/ee/observability"
	"github.com/kolide/launcher/v2/ee/tables/dataflattentable"
	"github.com/kolide/launcher/v2/ee/tables/tablehelpers"
	"github.com/kolide/launcher/v2/ee/tables/tablewrapper"
	"github.com/osquery/osquery-go/plugin/table"
)

// I've set 2 different states for if the signature is verified:
// VALID - The token parsed without errors and the signature was successfully validated.
// INVALID - The signature attempted to validate with the matched public key, but it was a bad key.
// By default the `verified` value is unset.
const (
	Valid   = "client_valid"
	Invalid = "invalid"
)

// Values for include_raw_jwt column.
var (
	allowedIncludeValues = []string{"true", "1"}
)

// Created errors here to handle switching the verified value depending on the returned error.
var (
	ErrMissingKeyId     = errors.New("no key id found in the JWT header")
	ErrMatchingKeyId    = errors.New("no key id matched the JWT header key id")
	ErrParsingPemBlock  = errors.New("error parsing PEM block containing the public key")
	ErrParsingPublicKey = errors.New("error parsing the public key from the PEM block")
)

type Table struct {
	slogger *slog.Logger
}

func TablePlugin(flags types.Flags, slogger *slog.Logger) *table.Plugin {
	columns := dataflattentable.Columns(
		table.TextColumn("path"),
		table.TextColumn("raw_data"),
		table.TextColumn("signing_keys"),
		table.TextColumn("include_raw_jwt"),
	)

	t := &Table{
		slogger: slogger.With("table", "kolide_jwt"),
	}

	return tablewrapper.New(flags, slogger, "kolide_jwt", columns, t.generate,
		tablewrapper.WithDescription("Parses JWTs and returns flattened claims and header data, with optional signature verification via signing keys. Requires at least one WHERE path or raw_data constraint."),
		tablewrapper.WithNote(dataflattentable.EAVNote),
	)
}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	ctx, span := observability.StartSpan(ctx, "table_name", "kolide_jwt")
	defer span.End()

	var results []map[string]string

	paths := tablehelpers.GetConstraints(queryContext, "path")
	rawDatas := tablehelpers.GetConstraints(queryContext, "raw_data")

	if len(paths) < 1 && len(rawDatas) < 1 {
		return nil, errors.New("kolide_jwt requires at least one path or raw_data to be specified")
	}

	for _, keyJSON := range tablehelpers.GetConstraints(queryContext, "signing_keys", tablehelpers.WithDefaults("")) {
		for _, includeRawJWT := range tablehelpers.GetConstraints(queryContext, "include_raw_jwt", tablehelpers.WithAllowedValues(allowedIncludeValues), tablehelpers.WithDefaults("false")) {
			for _, dataQuery := range tablehelpers.GetConstraints(queryContext, "query", tablehelpers.WithDefaults("*")) {
				for _, path := range paths {
					fileData, err := os.ReadFile(path)
					if len(fileData) == 0 || err != nil {
						t.slogger.Log(ctx, slog.LevelInfo, "error reading JWT data file", "err", err)
						continue
					}

					rawDataOutput := ""
					if includeRawJWT != "false" {
						rawDataOutput = string(fileData)
					}

					rowData := map[string]string{
						"path":            path,
						"raw_data":        rawDataOutput,
						"signing_keys":    keyJSON,
						"include_raw_jwt": includeRawJWT,
					}

					results = append(results, t.processJWT(ctx, fileData, keyJSON, dataQuery, rowData)...)
				}

				for _, rawData := range rawDatas {
					rowData := map[string]string{
						"path":            "",
						"raw_data":        rawData,
						"signing_keys":    keyJSON,
						"include_raw_jwt": includeRawJWT,
					}

					results = append(results, t.processJWT(ctx, []byte(rawData), keyJSON, dataQuery, rowData)...)
				}
			}
		}
	}

	return results, nil
}

func (t *Table) processJWT(ctx context.Context, rawData []byte, keyJSON string, dataQuery string, rowData map[string]string) []map[string]string {
	var keyMap map[string]string
	if err := json.Unmarshal([]byte(keyJSON), &keyMap); err != nil {
		t.slogger.Log(ctx, slog.LevelInfo, "error unmarshaling JWT signing keys", "err", err)
	}

	data := map[string]any{}
	token, err := jwt.ParseWithClaims(string(rawData), jwt.MapClaims{}, JWTKeyFunc(keyMap))
	if err != nil {
		t.slogger.Log(ctx, slog.LevelInfo, "error parsing token", "err", err)

		if errors.Is(err, ErrParsingPemBlock) || errors.Is(err, ErrParsingPublicKey) {
			data["verified"] = Invalid
		}
	} else {
		data["verified"] = Valid
	}

	if token == nil {
		return nil
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		t.slogger.Log(ctx, slog.LevelInfo, "error parsing JWT claims")
		return nil
	}

	parsedClaims := map[string]any{}
	maps.Copy(parsedClaims, claims)

	data["header"] = token.Header
	data["claims"] = parsedClaims

	flattenOpts := []dataflatten.FlattenOpts{
		dataflatten.WithSlogger(t.slogger),
		dataflatten.WithQuery(strings.Split(dataQuery, "/")),
	}

	flattened, err := dataflatten.Flatten(data, flattenOpts...)
	if err != nil {
		t.slogger.Log(ctx, slog.LevelInfo, "failure flattening JWT data", "err", err)
		return nil
	}

	return dataflattentable.ToMap(flattened, dataQuery, rowData)
}

// JWTKeyFunc handles taking in an array of public keys to validate against the JWT signature.
// There may be improvements by using `VerificationKeySet` to pass an array of crypto keys however, `VerificationKeySet` would require decoding the PEM block for each possible key instead of finding the correct key first.
func JWTKeyFunc(keys map[string]string) func(token *jwt.Token) (any, error) {
	return func(token *jwt.Token) (any, error) {
		// We may want to validate algorithm here alongside the key id.
		kid, ok := token.Header["kid"]
		if !ok {
			return nil, ErrMissingKeyId
		}

		for key_id, key := range keys {
			if key_id == kid {
				return JWTParsePublicKey(key)
			}
		}

		return nil, ErrMatchingKeyId
	}
}

// JWTParsePublicKey receives and decodes a public key string into a crypto PublicKey.
// This is required for jwt.Parse to have the correct data type for the public key.
func JWTParsePublicKey(key string) (crypto.PublicKey, error) {
	block, _ := pem.Decode([]byte(key))
	if block == nil {
		return nil, ErrParsingPemBlock
	}

	pubKey, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, ErrParsingPublicKey
	}

	return pubKey, nil
}
