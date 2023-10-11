package checkups

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"

	"github.com/golang-jwt/jwt/v5"
)

type enrollSecretCheckup struct {
	summary string
	status  Status
}

func (c *enrollSecretCheckup) Name() string {
	return "Enrollment Secret"
}

func (c *enrollSecretCheckup) Run(_ context.Context, extraFH io.Writer) error {
	secretStatus := make(map[string]Status, 0)
	secretSummary := make(map[string]string, 0)

	for _, secretPath := range getSecretPaths() {
		// Later on, we want to fall back to the _first_ secrets status. Set it here

		st, summary := parseSecret(extraFH, secretPath)
		secretStatus[secretPath] = st
		secretSummary[secretPath] = summary

		if c.status == Unknown || c.status == "" {
			c.status = st
			c.summary = summary
		}
	}

	// Iterate over all the found secrets. If any pass, this passes. Otherwise fall back to the first secret.
	// This is kinda roundabout, since this checkup is trying to support multiple possible paths
	if c.status == Passing {
		return nil
	}
	for secretPath, status := range secretStatus {
		if status == Passing {
			c.status = Passing
			c.summary = secretSummary[secretPath]
		}
	}

	if len(secretStatus) < 1 {
		c.status = Erroring
		c.summary = "No secrets for this platform"
		return nil
	}

	return nil
}

func (c *enrollSecretCheckup) ExtraFileName() string {
	return "secret-info.log"
}

func (c *enrollSecretCheckup) Status() Status {
	return c.status
}

func (c *enrollSecretCheckup) Summary() string {
	return c.summary
}

func (c *enrollSecretCheckup) Data() map[string]any {
	return nil
}

// getSecretPaths returns potential platform default secret path. It should probably get folded into flags, but I'm not
// quite sure how yet.
func getSecretPaths() []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{"/etc/kolide-k2/secret"}
	case "linux":
		return []string{"/etc/kolide-k2/secret"}
	case "windows":
		return []string{"C:\\Program Files\\Kolide\\Launcher-kolide-k2\\conf\\secret"}
	}

	return nil
}

func parseSecret(extraFH io.Writer, secretPath string) (Status, string) {
	fmt.Fprintf(extraFH, "%s:\n", secretPath)
	defer fmt.Fprintf(extraFH, "\n\n")

	secretFH, err := os.Open(secretPath)
	switch {
	case os.IsNotExist(err):
		fmt.Fprintf(extraFH, "does not exist\n")
		return Failing, "does not exist"
	case os.IsPermission(err):
		fmt.Fprintf(extraFH, "permission denied (might be running as user)\n")
		return Informational, "permission denied (might be running as user)"
	case err != nil:
		fmt.Fprintf(extraFH, "unknown error: %s\n", err)
		return Erroring, fmt.Sprintf("unknown error: %s", err)
	}
	defer secretFH.Close()

	// If we can read the secret, let's try to extract the munemo
	tokenRaw, err := io.ReadAll(secretFH)
	if err != nil {
		fmt.Fprintf(extraFH, "%s: unable to read: %s\n", secretPath, err)
		return Failing, fmt.Sprintf("unable to read: %s", err)
	}

	// We do not have the key, and thus CANNOT verify. So this is ParseUnverified
	token, _, err := new(jwt.Parser).ParseUnverified(string(tokenRaw), jwt.MapClaims{})
	if err != nil {
		fmt.Fprintf(extraFH, "Error parsing JWT:\n%s\n", err)
		return Failing, fmt.Sprintf("cannot jwt parse: %s", err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		fmt.Fprintf(extraFH, "no jwt claims\n")
		return Failing, "jwt has no claims"
	}

	// Print the claims into our extra data
	fmt.Fprintf(extraFH, "---\n")
	if err := json.NewEncoder(extraFH).Encode(claims); err != nil {
		fmt.Fprintf(extraFH, "Cannot json encode: %s\n", err)
	}
	fmt.Fprintf(extraFH, "---\n")

	// Expect the claims to have an organization
	org, ok := claims["organization"]
	if !ok {
		return Failing, "no organization in claim"
	}

	return Passing, fmt.Sprintf("claim for %s", org)
}
