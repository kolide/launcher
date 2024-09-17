package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"runtime"

	"github.com/kolide/launcher/ee/presencedetection"
	"github.com/kolide/launcher/pkg/log/multislogger"
)

func runDetectPresence(_ *multislogger.MultiSlogger, args []string) error {
	reason := ""

	if len(args) != 0 {
		reason = args[0]
	}

	if reason == "" && runtime.GOOS == "darwin" {
		return errors.New("reason is required on darwin")
	}

	success, err := presencedetection.Detect(reason)
	response := presencedetection.PresenceDetectionResponse{
		Success: success,
	}
	if err != nil {
		response.Error = err.Error()
	}

	// serialize response to JSON
	responseJSON, err := json.Marshal(response)
	if err != nil {
		return err
	}

	// b64 enode response
	responseB64 := base64.StdEncoding.EncodeToString(responseJSON)

	// write response to stdout
	if _, err := os.Stdout.Write([]byte(responseB64)); err != nil {
		return err
	}

	return nil
}
