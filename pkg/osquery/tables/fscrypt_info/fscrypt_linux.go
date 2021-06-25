// +build linux

package fscrypt_info

import (
	"io/ioutil"
	"log"

	"github.com/google/fscrypt/actions"
	"github.com/google/fscrypt/keyring"
	"github.com/google/fscrypt/metadata"
	"github.com/pkg/errors"
)

type Info struct {
	Encrypted      bool
	Locked         string
	Mountpoint     string
	FilesystemType string
	Device         string
	Path           string
	ContentsAlgo   string
	FilenameAlgo   string
}

func GetInfo(dirpath string) (*Info, error) {
	origLog := log.Writer()
	defer func() {
		log.SetOutput(origLog)
	}()
	log.SetOutput(ioutil.Discard)

	fsctx, err := actions.NewContextFromPath(dirpath, nil)
	if err != nil {
		return nil, errors.Wrap(err, "new context")
	}

	pol, err := actions.GetPolicyFromPath(fsctx, dirpath)
	switch err.(type) {
	case nil:
		return &Info{
			Path:           dirpath,
			Locked:         policyUnlockedStatus(pol),
			Encrypted:      true,
			Mountpoint:     pol.Context.Mount.Path,
			FilesystemType: pol.Context.Mount.FilesystemType,
			Device:         pol.Context.Mount.Device,
			ContentsAlgo:   pol.Options().Contents.String(),
			FilenameAlgo:   pol.Options().Filenames.String(),
		}, nil
	case *metadata.ErrNotEncrypted:
		return &Info{Path: dirpath, Encrypted: false}, nil
	default:
		return nil, errors.Wrapf(err, "get policy for %s", dirpath)
	}

	return nil, nil
}

// policyUnlockedStatus is from
// https://github.com/google/fscrypt/blob/dad0c1158455dcfd9acbd219a04ef348bf454332/cmd/fscrypt/status.go#L67
func policyUnlockedStatus(policy *actions.Policy) string {
	status := policy.GetProvisioningStatus()

	switch status {
	case keyring.KeyPresent, keyring.KeyPresentButOnlyOtherUsers:
		return "no"
	case keyring.KeyAbsent:
		return "yes"
	case keyring.KeyAbsentButFilesBusy:
		return "partially (incompletely locked)"
	default:
		return "unknown"
	}
}
