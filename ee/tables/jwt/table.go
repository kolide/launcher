package kolide_jwt

import (
	"context"
	"crypto"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/kolide/launcher/ee/dataflatten"
	"github.com/kolide/launcher/ee/tables/dataflattentable"
	"github.com/kolide/launcher/ee/tables/tablehelpers"
	"github.com/osquery/osquery-go/plugin/table"
)

// I've set 3 different states for if the signature is verified:
// VALID - The token parsed without errors and the signature was successfully validated.
// INVALID - The signature attempted to validate with the matched public key, but it was a bad key.
// UNKNOWN - The default state. This can mean that no key id matched, or simply no keys were provided to validate against.
const (
	Valid   = "VALID"
	Invalid = "INVALID"
	Unknown = "UNKNOWN"
)

// Values for include_raw_jwt column.
var (
	allowedIncludeValues = []string{"true", "false"}
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

func TablePlugin(slogger *slog.Logger) *table.Plugin {
	columns := dataflattentable.Columns(
		table.TextColumn("path"),
		table.TextColumn("signing_keys"),
		table.TextColumn("include_raw_jwt"),
	)

	t := &Table{
		slogger: slogger.With("table", "kolide_jwt"),
	}

	return table.NewPlugin("kolide_jwt", columns, t.generate)
}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	var results []map[string]string

	paths := tablehelpers.GetConstraints(queryContext, "path")
	if len(paths) < 1 {
		return nil, fmt.Errorf("kolide_jwt requires at least one path to be specified")
	}

	for _, path := range paths {
		for _, keyJSON := range tablehelpers.GetConstraints(queryContext, "signing_keys", tablehelpers.WithDefaults("")) {
			for _, include_raw_jwt := range tablehelpers.GetConstraints(queryContext, "include_raw_jwt", tablehelpers.WithAllowedValues(allowedIncludeValues), tablehelpers.WithDefaults("false")) {
				for _, dataQuery := range tablehelpers.GetConstraints(queryContext, "query", tablehelpers.WithDefaults("*")) {
					rawData, err := os.ReadFile(path)
					if err != nil {
						t.slogger.Log(ctx, slog.LevelInfo, "error reading JWT data file", "err", err)
						continue
					}

					// Parse provided JWT signing keys into an map for verification in the JWT parser
					var keyMap map[string]string
					if err := json.Unmarshal([]byte(keyJSON), &keyMap); err != nil {
						t.slogger.Log(ctx, slog.LevelInfo, "error unmarshaling JWT signing keys", "err", err)
					}

					row := map[string]interface{}{"verified": Unknown}
					token, err := jwt.ParseWithClaims(string(rawData), jwt.MapClaims{}, JWTKeyFunc(keyMap))
					if err != nil {
						t.slogger.Log(ctx, slog.LevelInfo, "error parsing token", "err", err)

						if errors.Is(err, ErrParsingPemBlock) || errors.Is(err, ErrParsingPublicKey) {
							row["verified"] = Invalid
						}
					} else {
						row["verified"] = Valid
					}

					claims, ok := token.Claims.(jwt.MapClaims)
					if !ok {
						t.slogger.Log(ctx, slog.LevelInfo, "error parsing JWT claims")
						continue
					}

					parsedClaims := map[string]interface{}{}
					for k, v := range claims {
						parsedClaims[k] = v
					}

					row["header"] = token.Header
					row["claims"] = parsedClaims

					if include_raw_jwt == "true" {
						row["raw_jwt"] = string(rawData)
					}

					flattenOpts := []dataflatten.FlattenOpts{
						dataflatten.WithSlogger(t.slogger),
						dataflatten.WithQuery(strings.Split(dataQuery, "/")),
					}

					flattened, err := dataflatten.Flatten(row, flattenOpts...)
					if err != nil {
						t.slogger.Log(ctx, slog.LevelInfo, "failure flattening JWT data", "err", err)
						continue
					}

					rowData := map[string]string{
						"path":            path,
						"signing_keys":    keyJSON,
						"include_raw_jwt": include_raw_jwt,
					}

					results = append(results, dataflattentable.ToMap(flattened, dataQuery, rowData)...)
				}
			}
		}
	}

	return results, nil
}

// JWTRaw takes a file path and returns the raw byte array. Nothing special here.
func JWTRaw(file string) ([]byte, error) {
	jwtFH, err := os.Open(file)
	if err != nil {
		return nil, fmt.Errorf("unable to access file: %w", err)
	}

	defer jwtFH.Close()

	tokenRaw, err := io.ReadAll(jwtFH)
	if err != nil {
		return nil, fmt.Errorf("unable to read JWT: %w", err)
	}

	return tokenRaw, nil
}

// JWTKeyFunc handles taking in an array of public keys to validate against the JWT signature.
// There may be improvements by using `VerificationKeySet` to pass an array of crypto keys however, `VerificationKeySet` would require decoding the PEM block for each possible key instead of finding the correct key first.
func JWTKeyFunc(keys map[string]string) func(token *jwt.Token) (interface{}, error) {
	return func(token *jwt.Token) (interface{}, error) {
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
