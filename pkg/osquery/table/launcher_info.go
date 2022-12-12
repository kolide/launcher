package table

import (
	"bytes"
	"context"
	"crypto/rsa"
	"fmt"
	"runtime"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/log/level"
	"github.com/kolide/kit/version"
	"github.com/kolide/krypto"
	"github.com/kolide/launcher/pkg/osquery"
	"github.com/kolide/launcher/pkg/osquery/runtime/history"
	"github.com/osquery/osquery-go/plugin/table"
	"go.etcd.io/bbolt"
)

func LauncherInfoTable(logger log.Logger, db *bbolt.DB) *table.Plugin {
	columns := []table.ColumnDefinition{
		table.TextColumn("branch"),
		table.TextColumn("build_date"),
		table.TextColumn("build_user"),
		table.TextColumn("go_version"),
		table.TextColumn("goarch"),
		table.TextColumn("goos"),
		table.TextColumn("revision"),
		table.TextColumn("version"),
		table.TextColumn("identifier"),
		table.TextColumn("osquery_instance_id"),
		table.TextColumn("fingerprint"),
		table.TextColumn("public_key"),
		table.TextColumn("hardware_public_encryption_key"),
		table.TextColumn("hardware_public_encryption_key_fingerprint"),
		table.TextColumn("hardware_public_signing_key"),
		table.TextColumn("hardware_public_signing_key_fingerprint"),
	}
	return table.NewPlugin("kolide_launcher_info", columns, generateLauncherInfoTable(logger, db))
}

var hardwarePublicEncryptionKey, hardwarePublicEncryptionKeyFingerprint, hardwarePublicSigningKey, hardwarePublicSigningKeyFingerprint string

func generateLauncherInfoTable(logger log.Logger, db *bbolt.DB) table.GenerateFunc {

	return func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		identifier, err := osquery.IdentifierFromDB(db)
		if err != nil {
			return nil, err
		}

		osqueryInstance, err := history.LatestInstance()
		if err != nil {
			return nil, err
		}

		publicKey, fingerprint, err := osquery.PublicKeyFromDB(db)
		if err != nil {
			// No logger here, so we can't easily log. Move on with blank values
			publicKey = ""
			fingerprint = ""
		}

		if runtime.GOOS == "windows" || runtime.GOOS == "linux" {
			if err := setHardwareKeys(&krypto.TpmEncoder{}); err != nil {
				level.Info(logger).Log(
					"msg", "TPM public keys not retreived",
					"err", err,
				)
			}
		}

		results := []map[string]string{
			{
				"branch":              version.Version().Branch,
				"build_date":          version.Version().BuildDate,
				"build_user":          version.Version().BuildUser,
				"go_version":          runtime.Version(),
				"goarch":              runtime.GOARCH,
				"goos":                runtime.GOOS,
				"revision":            version.Version().Revision,
				"version":             version.Version().Version,
				"identifier":          identifier,
				"osquery_instance_id": osqueryInstance.InstanceId,
				"fingerprint":         fingerprint,
				"public_key":          publicKey,
				// hardware encryption and signing keys refers to keys provided by either
				// Apple's secure enclave or Linux / Windows TPM
				"hardware_public_encryption_key":             hardwarePublicEncryptionKey,
				"hardware_public_encryption_key_fingerprint": hardwarePublicEncryptionKeyFingerprint,
				"hardware_public_signing_key":                hardwarePublicSigningKey,
				"hardware_public_signing_key_fingerprint":    hardwarePublicSigningKeyFingerprint,
			},
		}

		return results, nil
	}
}

type keyer interface {
	PublicSigningKey() (*rsa.PublicKey, error)
	PublicEncryptionKey() (*rsa.PublicKey, error)
}

func setHardwareKeys(keyer keyer) error {
	if hardwarePublicEncryptionKey != "" && hardwarePublicEncryptionKeyFingerprint != "" && hardwarePublicSigningKey != "" && hardwarePublicSigningKeyFingerprint != "" {
		return nil
	}

	// encryption key
	rsaEncryptionKey, err := keyer.PublicEncryptionKey()
	if err != nil {
		return fmt.Errorf("getting public encryption key from keyer: %w", err)
	}

	hardwarePublicEncryptionKey, err = keyToString(rsaEncryptionKey)
	if err != nil {
		return fmt.Errorf("converting public encryption key to string: %w", err)
	}

	hardwarePublicEncryptionKeyFingerprint, err = krypto.RsaFingerprint(rsaEncryptionKey)
	if err != nil {
		return fmt.Errorf("fingerprinting public encryption key: %w", err)
	}

	// singing key
	rsaSigningKey, err := keyer.PublicSigningKey()
	if err != nil {
		return fmt.Errorf("getting public signing key from keyer: %w", err)
	}

	hardwarePublicSigningKey, err = keyToString(rsaSigningKey)
	if err != nil {
		return fmt.Errorf("coverting public signing key to string: %w", err)
	}

	hardwarePublicSigningKeyFingerprint, err = krypto.RsaFingerprint(rsaSigningKey)
	if err != nil {
		return fmt.Errorf("fingerprinting public signing key: %w", err)
	}

	return nil
}

func keyToString(key *rsa.PublicKey) (string, error) {
	var b bytes.Buffer
	if err := krypto.RsaPublicToPublicPem(key, &b); err != nil {
		return "", fmt.Errorf("marshalling key to pem: %w", err)
	}

	return b.String(), nil
}
