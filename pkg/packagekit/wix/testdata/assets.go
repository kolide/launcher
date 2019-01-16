// Code generated by go-bindata.
// sources:
// testdata/assets/product.wxs
// DO NOT EDIT!

package testdata

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func bindataRead(data []byte, name string) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewBuffer(data))
	if err != nil {
		return nil, fmt.Errorf("Read %q: %v", name, err)
	}

	var buf bytes.Buffer
	_, err = io.Copy(&buf, gz)
	clErr := gz.Close()

	if err != nil {
		return nil, fmt.Errorf("Read %q: %v", name, err)
	}
	if clErr != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

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

var _testdataAssetsProductWxs = []byte("\x1f\x8b\x08\x00\x00\x00\x00\x00\x00\xff\x94\x93\x6f\x6f\xe2\x38\x10\xc6\x5f\xc7\x9f\x62\xce\xaa\x44\x5b\x09\x02\xd7\x0a\x55\x27\x0c\xe2\xa0\xf4\x50\xdb\x5b\x44\xe9\xf6\xb5\x89\x27\x89\xb5\x89\x1d\xd9\x0e\x0d\xdf\x7e\x95\x38\x50\xfa\x67\xa5\xdd\x77\x30\x19\x3f\xcf\xe3\xf9\x8d\x47\x93\x2a\xcf\x60\x87\xc6\x4a\xad\x18\x1d\xf4\xfa\x14\x50\x45\x5a\x48\x95\x30\xfa\xbc\x59\x74\x6f\xe8\x64\x4c\x46\x2f\xb2\x82\x2a\xcf\x94\x65\x34\x75\xae\xf8\x27\x0c\x6d\x94\x62\xce\x6d\x2f\x97\x91\xd1\x56\xc7\xae\x17\xe9\x3c\x7c\x95\x55\xf8\x77\xbf\x3f\x0c\x5f\x25\x1d\x13\x80\xd1\x44\xc6\x70\x76\x6e\xf7\xb6\xf7\xef\xf3\xf2\x61\x3e\x5d\xcf\xfe\xbb\x60\xb4\xba\x19\xd6\xb2\x00\x75\x87\xc0\x58\x2a\x84\x95\xd1\xc9\x42\x67\x02\x0d\xab\x7f\x1a\x9e\x2f\x64\x86\xd6\x97\x60\xe2\xd5\x30\xb3\xf8\xb5\xe2\xf0\xfa\x77\x15\x87\xd7\x9f\x35\x8f\x47\xd1\x18\x6d\xe0\x59\xd9\xb2\x28\xb4\x71\x28\x60\xc7\xb3\x12\x41\xc7\xf0\xce\x92\x7d\x8c\x70\x10\x53\x42\xc6\x93\x31\xa9\xff\xac\x8c\x16\x65\xe4\x60\x29\x18\xbd\xa4\x24\x00\x80\xff\x79\x8e\x8c\xd6\xd3\xdc\xa0\x75\xbe\xf6\xc0\x55\x52\xf2\x04\x19\x1d\xf4\xaf\xae\x7c\xed\xfb\x09\x91\x5e\xdf\xd7\x1e\xb9\x2a\x63\x1e\xb9\xd2\xa0\x01\x06\xf4\x5e\x67\x52\x20\x85\x31\x21\xc1\x68\xc5\xa3\x1f\x3c\x41\x12\x2c\x05\xeb\x5c\x76\x48\x70\x8f\xfb\x57\x6d\x84\x65\x9d\xa5\xb2\x8e\x67\x19\x9a\x0e\x09\xe6\x68\x23\x23\x0b\xd7\x68\x7b\x81\x26\x09\xbc\xc8\x8a\x92\x60\xa6\xf3\x1c\x95\xb3\x5f\x7d\x3b\xca\x1c\xb3\x5d\xf5\xfb\xfe\x4c\x61\xd0\x5a\x14\x8c\xee\xd1\xbe\x75\x3e\x45\xba\x40\x46\x0b\x34\x8f\x3c\x4a\xa5\x42\x4a\x82\xc3\x5d\x6d\x7b\x59\x08\x9b\x51\x01\x8c\xfe\xea\x76\x61\x93\x4a\x0b\xa9\xce\x84\x05\x97\x22\xc4\x35\x2d\x48\x50\xa1\xe1\x35\x89\xed\x1e\x52\xe4\x0e\xba\xdd\x96\xd6\x23\x0a\xc9\xeb\xf1\x76\x06\x1d\x98\xf1\xad\x54\xe8\x18\x4d\x74\x2f\xe2\x5b\x0a\xb7\xf9\x16\xc5\x8c\x6f\x7d\x2c\x38\xe4\x94\x5a\x3d\xe0\x0e\x33\x46\x53\x99\xa4\x27\x09\xe6\xd2\x60\xe4\xb4\xd9\x37\xc0\x36\xd3\xf5\xdd\xed\x66\xbe\x5c\xd3\x16\xda\x93\x2e\x4d\x84\x73\x69\xa8\x77\xff\x74\xe2\xec\x7c\xc7\x4d\xef\x6d\xe9\x2e\xe8\x98\x04\x1f\x7a\x56\xeb\x6f\x77\x27\x9a\x2d\xc2\xf0\xa8\x18\x1e\xdb\x7f\x61\x52\x13\xd2\x6a\x5a\x14\x73\xee\xb8\xf7\xf9\xc2\x66\x3e\xdd\x4c\xff\xc4\xe6\x5d\x81\x04\x0d\x8c\x96\xe2\x09\x89\xe3\xdc\x17\xc8\xeb\x2d\x6c\x96\x8d\xd6\x2b\xd2\xbc\x2b\x4a\x82\x8d\x74\x19\xfa\x52\x0d\xdb\x8f\x79\xf0\x36\xb0\x9a\x81\x56\xa8\xdc\x9d\xd1\x65\xb1\xc6\xb8\x49\x3b\x2d\x0a\x2f\x00\xe1\x21\x4e\xeb\xe0\xdf\x51\xd8\x3e\xa4\x31\x19\x85\x2f\xb2\x1a\x93\x9f\x01\x00\x00\xff\xff\x10\xf1\x1b\x03\xb6\x04\x00\x00")

func testdataAssetsProductWxsBytes() ([]byte, error) {
	return bindataRead(
		_testdataAssetsProductWxs,
		"testdata/assets/product.wxs",
	)
}

func testdataAssetsProductWxs() (*asset, error) {
	bytes, err := testdataAssetsProductWxsBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "testdata/assets/product.wxs", size: 1206, mode: os.FileMode(420), modTime: time.Unix(1547617184, 0)}
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
	"testdata/assets/product.wxs": testdataAssetsProductWxs,
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
	"testdata": &bintree{nil, map[string]*bintree{
		"assets": &bintree{nil, map[string]*bintree{
			"product.wxs": &bintree{testdataAssetsProductWxs, map[string]*bintree{}},
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

