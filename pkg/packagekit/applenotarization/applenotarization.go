// Package applenotarization is a wrapper around the apple
// notarization tools.
//
// It supports submitting to apple, and recording the uuid. As well as
// checking on the status.
package applenotarization

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
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

var duplicateSubmitRegexp = regexp.MustCompile(`ERROR ITMS-90732: "The software asset has already been uploaded. The upload ID is ([0-9a-fA-F-]+)"`)

var retryableErrorStrings = []string{
	"Message:Unable to find requested file(s): metadata.xml",
}

const (
	maxSubmitTries = 5
)

// Submit an file to apple's notarization service. Returns the uuid of
// the submission
func (n *Notarizer) Submit(ctx context.Context, filePath string, primaryBundleId string) (string, error) {
	logger := log.With(ctxlog.FromContext(ctx), "caller", "applenotarization.Submit")

	// TODO check file extension here
	// zip,pkg,dmg

	// Sometimes submit fails. Retry a couple times.
	for attempt := 1; attempt <= maxSubmitTries; attempt++ {

		// According to a support thread on apple, the bundle-id passed to
		// altool's notarize is unrelated to the actual software. It is only
		// used to for something in local storage. As such, add a nonce to
		// ensure nothing is conflicting.
		// https://developer.apple.com/forums/thread/677739
		bundleId := primaryBundleId + "." + randomNonce()

		response, err := n.runAltool(ctx, []string{
			"--notarize-app",
			"--primary-bundle-id", bundleId,
			"--file", filePath,
		})

		if err != nil {
			level.Error(logger).Log(
				"msg", "error getting notarize-app. Will retry",
				"attempt", attempt,
				"err", err,
				"error-messages", fmt.Sprintf("%+v", response.ProductErrors),
			)
			continue
		}

		// retryable errors
		if len(response.ProductErrors) > 0 {
			for _, e := range retryableErrorStrings {
				if strings.Contains(response.ProductErrors[0].Message, e) {
					level.Error(logger).Log(
						"msg", "error submitting for notarization. Will retry",
						"attempt", attempt,
						"error-messages", fmt.Sprintf("%+v", response.ProductErrors),
					)
					continue
				}
			}
		}

		// duplicate submission err, treat as success.
		if len(response.ProductErrors) == 1 {
			matches := duplicateSubmitRegexp.FindStringSubmatch(response.ProductErrors[0].Message)
			if len(matches) == 2 {
				return matches[1], nil
			}
		}

		if err == nil {
			return response.NotarizationUpload.RequestUUID, nil
		}
	}

	// Falling out of the for loop means we never succeeded
	return "", errors.New("Did not successfully submit for notarization")
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

// randomNonce returns a short random hex string.
func randomNonce() string {
	buff := make([]byte, 3)
	rand.Read(buff)
	str := hex.EncodeToString(buff)
	return str
}
