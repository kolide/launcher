// Code generated by go-bindata.
// sources:
// internal/assets/main.wxs
// DO NOT EDIT!

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

var _internalAssetsMainWxs = []byte(`<?xml version="1.0" encoding="UTF-8"?>
<Wix xmlns="http://schemas.microsoft.com/wix/2006/wi">
<?if $(sys.BUILDARCH)="x86"?>
    <?define Minimum_Version="100"?>
    <?define Program_Files="ProgramFilesFolder"?>
<?elseif $(sys.BUILDARCH)="x64"?>
    <?define Minimum_Version="200"?>
    <?define Program_Files="ProgramFiles64Folder"?>
<?else?>
    <?error Unsupported value of sys.BUILDARCH=$(sys.BUILDARCH)?>
<?endif?>

  <Product
      Id="*"
      Name="Kolide {{.Opts.Name}} {{.Opts.Identifier}} $(sys.BUILDARCH) {{.Opts.Version}}"
      Language="1033"
      Version="{{.Opts.Version}}"
      Manufacturer="https://kolide.com"
      UpgradeCode="{{.UpgradeCode}}" >

    <Package
	Id='*'
	Keywords='Installer'
	Description="Kolide {{.Opts.Name}} {{.Opts.Identifier}}"
	Comments="Kolide {{.Opts.Name}} {{.Opts.Identifier}}"
	InstallerVersion="300"
	Compressed="yes"
	InstallScope="perMachine"
	Languages="1033" />

    <!-- This holds the files generated by heat -->
    <Media Id='1' Cabinet="go.cab" EmbedCab="yes" CompressionLevel="high" />

    <MajorUpgrade AllowDowngrades="yes" />

    <!-- Set INSTALLDIR to "c:\Program Files\Kolide". With appropriate
	 WiX replacements along the way
	 Set DATADIR to "c:\ProgramData\Kolide"

	 It's up to the caller to decide which directories to use
    -->
    <Directory Id="TARGETDIR" Name="SourceDir">
      <Directory Id="$(var.Program_Files)">
	<Directory Id="PROGDIR" Name="Kolide"/>
      </Directory>
      <Directory Id="CommonAppDataFolder">
	<Directory Id="DATADIR" Name="Kolide"/>
      </Directory>
    </Directory>

    <!-- Install the files -->
    <Feature
	Id="LauncherFiles"
	Title="Launcher"
	Level="1">
      <ComponentGroupRef Id="AppFiles" />
    </Feature>

  </Product>
</Wix>
`)

func internalAssetsMainWxsBytes() ([]byte, error) {
	return _internalAssetsMainWxs, nil
}

func internalAssetsMainWxs() (*asset, error) {
	bytes, err := internalAssetsMainWxsBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "internal/assets/main.wxs", size: 1750, mode: os.FileMode(420), modTime: time.Unix(1549050131, 0)}
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
	"internal/assets/main.wxs": internalAssetsMainWxs,
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
			"main.wxs": &bintree{internalAssetsMainWxs, map[string]*bintree{}},
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

