// Package applenotarization is a wrapper around the apple
// notarization tools.
//
// It supports submitting to apple, and recording the uuid. As well as
// checking on the status.
package applenotarization

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/groob/plist"
	"github.com/kolide/launcher/pkg/contexts/ctxlog"
	"github.com/pkg/errors"
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

var duplicateSubmitRegexp = regexp.MustCompile(`ERROR ITMS-90732: "The software asset has already been uploaded. The upload ID is ([0-9a-fA-F-]+)"`)

// Submit an file to apple's notarization service. Returns the uuid of
// the submission
func (n *Notarizer) Submit(ctx context.Context, filePath string, primaryBundleId string) (string, error) {
	logger := log.With(ctxlog.FromContext(ctx), "caller", "applenotarization.Submit")

	// TODO check file extension here
	// zip,pkg,dmg

	response, err := n.runAltool(ctx, []string{
		"--notarize-app",
		"--primary-bundle-id", primaryBundleId,
		"--file", filePath,
	})

	// duplicate submissions
	if len(response.ProductErrors) == 1 {
		matches := duplicateSubmitRegexp.FindStringSubmatch(response.ProductErrors[0].Message)
		if len(matches) == 2 {
			return matches[1], nil
		}
	}

	if err != nil {
		level.Error(logger).Log(
			"msg", "error getting notarize-app",
			"error-messages", fmt.Sprintf("%+v", response.ProductErrors),
		)

		return "", errors.Wrap(err, "calling notarize")
	}

	return response.NotarizationUpload.RequestUUID, nil
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
		return "", errors.Wrap(err, "exec")
	}

	if response.NotarizationInfo.RequestUUID != uuid {
		return "", errors.Errorf("Something went wrong. Expected response for %s, but got %s",
			response.NotarizationInfo.RequestUUID,
			uuid,
		)
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
			return nil, errors.Wrap(err, "plist decode")
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
		return nil, errors.Wrap(err, "plist decode")
	}

	return response, cmdErr
}
