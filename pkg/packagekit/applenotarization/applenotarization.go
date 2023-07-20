// Package applenotarization is a wrapper around the apple
// notarization tools.
//
// It supports submitting to apple, and recording the uuid. As well as
// checking on the status.
package applenotarization

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/groob/plist"
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
	return n.runNotarytool(ctx, filePath)
}

// Check the notarization status of a uuid
func (n *Notarizer) Check(ctx context.Context, uuid string) (string, error) {
	logger := log.With(ctxlog.FromContext(ctx),
		"caller", "applenotarization.Check",
		"request-uuid", uuid,
	)

	response, err := n.runAltool(ctx, []string{"--notarization-info", uuid})
	if err != nil {
		level.Error(logger).Log(
			"msg", "error getting notarization-info",
			"error-messages", fmt.Sprintf("%+v", response.ProductErrors),
		)
		return "", fmt.Errorf("exec: %w", err)
	}

	if response.NotarizationInfo.RequestUUID != uuid {
		return "", fmt.Errorf("Something went wrong. Expected response for %s, but got %s",
			response.NotarizationInfo.RequestUUID,
			uuid)

	}

	if response.NotarizationInfo.Status != "success" {
		level.Info(logger).Log(
			"msg", "Not successful. Examine log",
			"logfile", response.NotarizationInfo.LogFileURL,
		)
	}

	return response.NotarizationInfo.Status, nil
}

func Staple(ctx context.Context) {
}

func (n *Notarizer) runNotarytool(ctx context.Context, file string) (string, error) {
	logger := log.With(ctxlog.FromContext(ctx), "caller", "applenotarization.runNotarytool")

	baseArgs := []string{
		"notarytool",
		"submit",
		file,
		"--apple-id", n.username,
		"--password", n.password,
		"--team-id", n.account,
		"--output-format", "json",
		"--no-wait",
		"--timeout", "3m",
	}

	cmd := exec.CommandContext(ctx, "xcrun", baseArgs...)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("notarizing error: error `%w`, output `%s`", err, string(out))
	}

	type notarizationResponse struct {
		Message string `json:"message"`
		ID      string `json:"id"`
		Path    string `json:"path"`
	}
	var r notarizationResponse
	if err := json.Unmarshal(out, &r); err != nil {
		return "", fmt.Errorf("could not unmarshal notarization response: %w", err)
	}

	level.Debug(logger).Log(
		"msg", "successfully submitted for notarization",
		"response_msg", r.Message,
		"response_uuid", r.ID,
		"response_path", r.Path,
	)

	return r.ID, nil
}

func (n *Notarizer) runAltool(ctx context.Context, cmdArgs []string) (*notarizationResponse, error) {
	logger := log.With(ctxlog.FromContext(ctx), "caller", "applenotarization.runAltool")

	baseArgs := []string{
		"altool",
		"--username", n.username,
		"--password", "@env:N_PASS",
		"--asc-provider", n.account,
		"--output-format", "xml",
	}

	cmd := exec.CommandContext(ctx, "xcrun", append(baseArgs, cmdArgs...)...)
	cmd.Env = append(cmd.Env, fmt.Sprintf("N_PASS=%s", n.password))

	level.Debug(logger).Log(
		"msg", "Execing altool as",
		"cmd", strings.Join(cmd.Args, " "),
	)

	if n.fakeResponse != "" {
		response := &notarizationResponse{}
		if err := plist.NewXMLDecoder(strings.NewReader(n.fakeResponse)).Decode(response); err != nil {
			return nil, fmt.Errorf("plist decode: %w", err)
		}

		// This isn't quite right -- we're returng nil, and
		// not the command error. But it's good enough...
		return response, nil
	}

	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	cmd.Stdout, cmd.Stderr = stdout, stderr
	cmdErr := cmd.Run()

	// So far, we get xml output even in the face of errors. So we may as well try to parse it here.
	response := &notarizationResponse{}
	if err := plist.NewXMLDecoder(stdout).Decode(response); err != nil {
		return nil, fmt.Errorf("plist decode: %w", err)
	}

	return response, cmdErr
}
