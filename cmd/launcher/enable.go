package main

import (
	"net/http"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/autoupdate"
	"github.com/kolide/updater/tuf"
)

type updateEnabler struct {
	Logger             log.Logger
	RootDirectory      string
	AutoupdateInterval time.Duration

	NotaryURL string
	MirrorURL string

	HttpClient *http.Client
}

func (e *updateEnabler) EnableBinary(binaryPath string, opts ...autoupdate.UpdaterOption) (stop func(), err error) {
	completeOpts := []autoupdate.UpdaterOption{
		autoupdate.WithLogger(e.Logger),
		autoupdate.WithHTTPClient(e.HttpClient),
		autoupdate.WithNotaryURL(e.NotaryURL),
		autoupdate.WithMirrorURL(e.MirrorURL),
	}
	completeOpts = append(completeOpts, opts...)
	updater, err := autoupdate.NewUpdater(
		binaryPath,
		e.RootDirectory,
		e.Logger,
		completeOpts...,
	)
	if err != nil {
		return nil, err
	}

	return updater.Run(tuf.WithFrequency(e.AutoupdateInterval))
}
