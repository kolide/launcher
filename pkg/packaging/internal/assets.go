// Code generated by go-bindata. DO NOT EDIT.
// sources:
// internal/assets/installerinfo.sh
// internal/assets/postinstall-init.sh
// internal/assets/postinstall-launchd.sh
// internal/assets/postinstall-systemd.sh
// internal/assets/postinstall-upstart.sh
package internal

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"
)
type asset struct {
	bytes []byte
	info  os.FileInfo
}

type bindataFileInfo struct {
	name    string
	size    int64
	mode    os.FileMode
	modTime time.Time
}

func (fi bindataFileInfo) Name() string {
	return fi.name
}
func (fi bindataFileInfo) Size() int64 {
	return fi.size
}
func (fi bindataFileInfo) Mode() os.FileMode {
	return fi.mode
}
func (fi bindataFileInfo) ModTime() time.Time {
	return fi.modTime
}
func (fi bindataFileInfo) IsDir() bool {
	return false
}
func (fi bindataFileInfo) Sys() interface{} {
	return nil
}

var _internalAssetsInstallerinfoSh = []byte(`{{- /* As this is shared between the installers, this is a reused template fragment */ -}}
cat <<EOF > "{{.InfoOutput}}"
{
  "identifier": "{{Identifier}}",
  "installer_id": "$INSTALL_PKG_SESSION_ID",
  "installer_path": "$PACKAGE_PATH",
  "timestamp": "$(date +%Y-%m-%dT%T%z)",
  "version": "{{.Version}}"
}
EOF
`)

func internalAssetsInstallerinfoShBytes() ([]byte, error) {
	return _internalAssetsInstallerinfoSh, nil
}

func internalAssetsInstallerinfoSh() (*asset, error) {
	bytes, err := internalAssetsInstallerinfoShBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "internal/assets/installerinfo.sh", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _internalAssetsPostinstallInitSh = []byte(`#!/bin/sh

if [ ! -z "{{.InfoOutput}}" ]; then
    cat <<EOF > "{{.InfoOutput}}"
{{.InfoJson}}
 EOF
fi

sudo service launcher.{{.Identifier}} restart
`)

func internalAssetsPostinstallInitShBytes() ([]byte, error) {
	return _internalAssetsPostinstallInitSh, nil
}

func internalAssetsPostinstallInitSh() (*asset, error) {
	bytes, err := internalAssetsPostinstallInitShBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "internal/assets/postinstall-init.sh", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _internalAssetsPostinstallLaunchdSh = []byte(`#!/bin/sh

[[ $3 != "/" ]] && exit 0

/bin/launchctl stop com.{{.Identifier}}.launcher

if [ ! -z "{{.InfoOutput}}" ]; then
    cat <<EOF > "{{.InfoOutput}}"
{{.InfoJson}}
EOF

    plutil -convert xml1 -o  "{{StringsTrimSuffix .InfoOutput `+"`"+`.json`+"`"+`}}.xml" "{{.InfoOutput}}"
fi

# Sleep to let the stop take effect
sleep 5

/bin/launchctl unload {{.Path}}
/bin/launchctl load {{.Path}}
`)

func internalAssetsPostinstallLaunchdShBytes() ([]byte, error) {
	return _internalAssetsPostinstallLaunchdSh, nil
}

func internalAssetsPostinstallLaunchdSh() (*asset, error) {
	bytes, err := internalAssetsPostinstallLaunchdShBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "internal/assets/postinstall-launchd.sh", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _internalAssetsPostinstallSystemdSh = []byte(`#!/bin/sh

if [ ! -z "{{.InfoOutput}}" ]; then
    cat <<EOF > "{{.InfoOutput}}"
{{.InfoJson}}
EOF
fi

set -e

systemctl daemon-reload

systemctl enable launcher.{{.Identifier}}
systemctl restart launcher.{{.Identifier}}
`)

func internalAssetsPostinstallSystemdShBytes() ([]byte, error) {
	return _internalAssetsPostinstallSystemdSh, nil
}

func internalAssetsPostinstallSystemdSh() (*asset, error) {
	bytes, err := internalAssetsPostinstallSystemdShBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "internal/assets/postinstall-systemd.sh", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _internalAssetsPostinstallUpstartSh = []byte(`#!/bin/sh

# upstart's stop and restart commands error out if the daemon isn't
# running. So stop and start are separate, and `+"`"+`set -e`+"`"+` is after the
# stop.

if [ ! -z "{{.InfoOutput}}" ]; then
    cat <<EOF > "{{.InfoOutput}}"
{{.InfoJson}}
EOF
fi

stop launcher-{{.Identifier}}
set -e
start launcher-{{.Identifier}}
`)

func internalAssetsPostinstallUpstartShBytes() ([]byte, error) {
	return _internalAssetsPostinstallUpstartSh, nil
}

func internalAssetsPostinstallUpstartSh() (*asset, error) {
	bytes, err := internalAssetsPostinstallUpstartShBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "internal/assets/postinstall-upstart.sh", size: 0, mode: os.FileMode(0), modTime: time.Unix(0, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

// Asset loads and returns the asset for the given name.
// It returns an error if the asset could not be found or
// could not be loaded.
func Asset(name string) ([]byte, error) {
	cannonicalName := strings.Replace(name, "\\", "/", -1)
	if f, ok := _bindata[cannonicalName]; ok {
		a, err := f()
		if err != nil {
			return nil, fmt.Errorf("Asset %s can't read by error: %v", name, err)
		}
		return a.bytes, nil
	}
	return nil, fmt.Errorf("Asset %s not found", name)
}

// MustAsset is like Asset but panics when Asset would return an error.
// It simplifies safe initialization of global variables.
func MustAsset(name string) []byte {
	a, err := Asset(name)
	if err != nil {
		panic("asset: Asset(" + name + "): " + err.Error())
	}

	return a
}

// AssetInfo loads and returns the asset info for the given name.
// It returns an error if the asset could not be found or
// could not be loaded.
func AssetInfo(name string) (os.FileInfo, error) {
	cannonicalName := strings.Replace(name, "\\", "/", -1)
	if f, ok := _bindata[cannonicalName]; ok {
		a, err := f()
		if err != nil {
			return nil, fmt.Errorf("AssetInfo %s can't read by error: %v", name, err)
		}
		return a.info, nil
	}
	return nil, fmt.Errorf("AssetInfo %s not found", name)
}

// AssetNames returns the names of the assets.
func AssetNames() []string {
	names := make([]string, 0, len(_bindata))
	for name := range _bindata {
		names = append(names, name)
	}
	return names
}

// _bindata is a table, holding each asset generator, mapped to its name.
var _bindata = map[string]func() (*asset, error){
	"internal/assets/installerinfo.sh": internalAssetsInstallerinfoSh,
	"internal/assets/postinstall-init.sh": internalAssetsPostinstallInitSh,
	"internal/assets/postinstall-launchd.sh": internalAssetsPostinstallLaunchdSh,
	"internal/assets/postinstall-systemd.sh": internalAssetsPostinstallSystemdSh,
	"internal/assets/postinstall-upstart.sh": internalAssetsPostinstallUpstartSh,
}

// AssetDir returns the file names below a certain
// directory embedded in the file by go-bindata.
// For example if you run go-bindata on data/... and data contains the
// following hierarchy:
//     data/
//       foo.txt
//       img/
//         a.png
//         b.png
// then AssetDir("data") would return []string{"foo.txt", "img"}
// AssetDir("data/img") would return []string{"a.png", "b.png"}
// AssetDir("foo.txt") and AssetDir("notexist") would return an error
// AssetDir("") will return []string{"data"}.
func AssetDir(name string) ([]string, error) {
	node := _bintree
	if len(name) != 0 {
		cannonicalName := strings.Replace(name, "\\", "/", -1)
		pathList := strings.Split(cannonicalName, "/")
		for _, p := range pathList {
			node = node.Children[p]
			if node == nil {
				return nil, fmt.Errorf("Asset %s not found", name)
			}
		}
	}
	if node.Func != nil {
		return nil, fmt.Errorf("Asset %s not found", name)
	}
	rv := make([]string, 0, len(node.Children))
	for childName := range node.Children {
		rv = append(rv, childName)
	}
	return rv, nil
}

type bintree struct {
	Func     func() (*asset, error)
	Children map[string]*bintree
}
var _bintree = &bintree{nil, map[string]*bintree{
	"internal": &bintree{nil, map[string]*bintree{
		"assets": &bintree{nil, map[string]*bintree{
			"installerinfo.sh": &bintree{internalAssetsInstallerinfoSh, map[string]*bintree{}},
			"postinstall-init.sh": &bintree{internalAssetsPostinstallInitSh, map[string]*bintree{}},
			"postinstall-launchd.sh": &bintree{internalAssetsPostinstallLaunchdSh, map[string]*bintree{}},
			"postinstall-systemd.sh": &bintree{internalAssetsPostinstallSystemdSh, map[string]*bintree{}},
			"postinstall-upstart.sh": &bintree{internalAssetsPostinstallUpstartSh, map[string]*bintree{}},
		}},
	}},
}}

// RestoreAsset restores an asset under the given directory
func RestoreAsset(dir, name string) error {
	data, err := Asset(name)
	if err != nil {
		return err
	}
	info, err := AssetInfo(name)
	if err != nil {
		return err
	}
	err = os.MkdirAll(_filePath(dir, filepath.Dir(name)), os.FileMode(0755))
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(_filePath(dir, name), data, info.Mode())
	if err != nil {
		return err
	}
	err = os.Chtimes(_filePath(dir, name), info.ModTime(), info.ModTime())
	if err != nil {
		return err
	}
	return nil
}

// RestoreAssets restores an asset under the given directory recursively
func RestoreAssets(dir, name string) error {
	children, err := AssetDir(name)
	// File
	if err != nil {
		return RestoreAsset(dir, name)
	}
	// Dir
	for _, child := range children {
		err = RestoreAssets(dir, filepath.Join(name, child))
		if err != nil {
			return err
		}
	}
	return nil
}

func _filePath(dir, name string) string {
	cannonicalName := strings.Replace(name, "\\", "/", -1)
	return filepath.Join(append([]string{dir}, strings.Split(cannonicalName, "/")...)...)
}

