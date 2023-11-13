// Package applenotarization is a wrapper around the apple
// notarization tools.
//
// It supports submitting to apple, and recording the uuid. As well as
// checking on the status.
package applenotarization

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/contexts/ctxlog"
)

type Notarizer struct {
	username     string // apple id
	password     string // application password
	account      string // This is the itunes account identifier
	fakeResponse string
}

func New(
	username string,
	password string,
	account string,
) *Notarizer {
	n := &Notarizer{
		username: username,
		password: password,
		account:  account,
	}

	return n

}

// Submit an file to apple's notarization service. Returns the uuid of
// the submission
func (n *Notarizer) Submit(ctx context.Context, filePath string, primaryBundleId string) (string, error) {
	rawResp, err := n.runNotarytool(ctx, "submit", filePath, []string{"--no-wait", "--timeout", "3m"})
	if err != nil {
		return "", fmt.Errorf("could not run notarytool submit: %w", err)
	}

	var r notarizationResponse
	if err := json.Unmarshal(rawResp, &r); err != nil {
		return "", fmt.Errorf("could not unmarshal notarization response: %w", err)
	}

	return r.ID, nil
}

// Check the notarization status of a uuid
func (n *Notarizer) Check(ctx context.Context, uuid string) (string, error) {
	logger := log.With(ctxlog.FromContext(ctx),
		"caller", "applenotarization.Check",
		"request-uuid", uuid,
	)

	rawResp, err := n.runNotarytool(ctx, "info", uuid, nil)
	if err != nil {
		return "", fmt.Errorf("fetching notarization info: %w", err)
	}

	var r notarizationInfoResponse
	if err := json.Unmarshal(rawResp, &r); err != nil {
		return "", fmt.Errorf("could not unmarshal notarization info response: %w", err)
	}

	if r.ID != uuid {
		return "", fmt.Errorf("something went wrong. Expected response for %s, but got %s", r.ID, uuid)
	}

	if r.Status != "Accepted" {
		level.Info(logger).Log(
			"msg", "Not successful. Examine log",
			"status", r.Status,
		)
	}

	return r.Status, nil
}

func Staple(ctx context.Context) {
}

func (n *Notarizer) runNotarytool(ctx context.Context, command string, target string, additionalArgs []string) ([]byte, error) {
	baseArgs := []string{
		"notarytool",
		command,
		target,
		"--apple-id", n.username,
		"--password", n.password,
		"--team-id", n.account,
		"--output-format", "json",
	}
	if len(additionalArgs) > 0 {
		baseArgs = append(baseArgs, additionalArgs...)
	}

	if n.fakeResponse != "" {
		return []byte(n.fakeResponse), nil
	}

	cmd := exec.CommandContext(ctx, "xcrun", baseArgs...) //nolint:forbidigo // Fine to use exec.CommandContext outside of launcher proper

	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("notarizing error: error `%w`, output `%s`", err, string(out))
	}

	return out, nil
}
