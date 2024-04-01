//go:build darwin
// +build darwin

package localserver

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/asn1"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kolide/launcher/ee/agent"
	"github.com/kolide/launcher/ee/allowedcmd"
	"howett.net/plist"
)

func addCertToKeyStore(ctx context.Context, certRaw []byte, cert *x509.Certificate) error {
	// TODO make a cert manager, make a real location for the cert, make sure it gets cleaned up, etc
	certDir, err := agent.MkdirTemp("launcher-cert")
	if err != nil {
		return fmt.Errorf("making temp dir: %w", err)
	}

	// Write out the cert
	certFilepath := filepath.Join(certDir, "kolide-localhost.crt")
	if err := os.WriteFile(certFilepath, certRaw, 0644); err != nil {
		return fmt.Errorf("writing out cert file: %w", err)
	}

	// Add the cert
	addTrustedCertCmd, err := allowedcmd.Security(ctx, "add-trusted-cert", "-d", "-r", "trustRoot", "-k", "/Library/Keychains/System.keychain", certFilepath)
	if err != nil {
		return fmt.Errorf("creating security add-trusted-cert command: %w", err)
	}
	if out, err := addTrustedCertCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("adding trusted cert: output `%s`: %w", string(out), err)
	}

	// Prepare updated trust settings
	newTrustSettingsRaw := []byte(`
<array>
	<dict>
		<key>kSecTrustSettingsPolicy</key>
		<data>
		KoZIhvdjZAED
		</data>
		<key>kSecTrustSettingsPolicyName</key>
		<string>sslServer</string>
		<key>kSecTrustSettingsResult</key>
		<integer>1</integer>
	</dict>
	<dict>
		<key>kSecTrustSettingsPolicy</key>
		<data>
		KoZIhvdjZAEC
		</data>
		<key>kSecTrustSettingsPolicyName</key>
		<string>basicX509</string>
		<key>kSecTrustSettingsResult</key>
		<integer>1</integer>
	</dict>
</array>
`)
	var newTrustSettings []interface{}
	if _, err := plist.Unmarshal(newTrustSettingsRaw, &newTrustSettings); err != nil {
		return fmt.Errorf("unmarshalling new trust settings: %w", err)
	}

	// Read in the trust settings so that we can overwrite them for our cert
	trustSettingsPlistFilename := filepath.Join(certDir, "trust-settings")
	defer os.Remove(trustSettingsPlistFilename)

	// -d (admin trust settings)
	trustSettingsExportCmd, err := allowedcmd.Security(ctx, "trust-settings-export", "-d", trustSettingsPlistFilename)
	if err != nil {
		return fmt.Errorf("creating security trust-settings-export command")
	}
	if out, err := trustSettingsExportCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("exporting trust settings: output `%s`: %w", string(out), err)
	}

	trustSettingsPlistRaw, err := os.ReadFile(trustSettingsPlistFilename)
	if err != nil {
		return fmt.Errorf("reading trust settings from file %s: %w", trustSettingsPlistFilename, err)
	}

	var trustSettingsPlist map[string]interface{}
	if _, err := plist.Unmarshal(trustSettingsPlistRaw, &trustSettingsPlist); err != nil {
		return fmt.Errorf("parsing trust settings: %w", err)
	}

	// TODO check/validate plist contents before accessing
	trustList := trustSettingsPlist["trustList"].(map[string]interface{})
	rootSubjectASN1, _ := asn1.Marshal(cert.Subject.ToRDNSequence())
	for key := range trustList {
		entry := trustList[key].(map[string]interface{})
		if _, ok := entry["issuerName"]; !ok {
			continue
		}
		issuerName := entry["issuerName"].([]byte)
		if !bytes.Equal(rootSubjectASN1, issuerName) {
			continue
		}
		entry["trustSettings"] = newTrustSettings
		break
	}

	updatedPlistRaw, err := plist.MarshalIndent(trustSettingsPlist, plist.XMLFormat, "\t")
	if err != nil {
		return fmt.Errorf("marshalling updated trust settings: %w", err)
	}

	updatedTrustSettingsPlistFilename := filepath.Join(certDir, "updated-trust-settings")
	defer os.Remove(updatedTrustSettingsPlistFilename)
	if err := os.WriteFile(updatedTrustSettingsPlistFilename, updatedPlistRaw, 0644); err != nil {
		return fmt.Errorf("writing out updated trust settings to %s: %w", updatedTrustSettingsPlistFilename, err)
	}

	trustSettingsImportCmd, err := allowedcmd.Security(ctx, "trust-settings-import", "-d", updatedTrustSettingsPlistFilename)
	if err != nil {
		return fmt.Errorf("creating security trust-settings-import command")
	}
	if out, err := trustSettingsImportCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("importing updated trust settings: output `%s`: %w", string(out), err)
	}

	return nil
}
