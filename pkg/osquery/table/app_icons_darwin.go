package table

/*
#cgo darwin CFLAGS: -DDARWIN -x objective-c
#cgo darwin LDFLAGS: -framework Cocoa
#include <CoreFoundation/CoreFoundation.h>
#import <Collaboration/Collaboration.h>
void Icon(CFDataRef *iconDataRef, char* path) {
NSString *iconPath = [[NSString alloc] initWithUTF8String:path];

if (![[NSFileManager defaultManager] fileExistsAtPath:iconPath]) {
 return;
}

NSImage *img = [[NSImage alloc] initWithContentsOfFile:iconPath];
NSArray *reps = [img representations];
NSBitmapImageRep *largestRep;

for (NSImageRep *rep in reps) {
	if (![rep isKindOfClass:[NSBitmapImageRep class]]) {
  	continue;
  }
 NSBitmapImageRep *brep = (NSBitmapImageRep *)rep;
 if ([brep pixelsWide] >= 128) {
 	NSData *imageData = [brep TIFFRepresentation];
	*iconDataRef = (CFDataRef)imageData;
	break;
 }
}
}
*/
import "C"
import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"os"
	"path"
	"unsafe"

	"github.com/go-kit/kit/log"
	"github.com/groob/plist"
	osquery "github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"
	"github.com/nfnt/resize"
	"github.com/pkg/errors"
	"golang.org/x/image/tiff"
)

func AppIcons(client *osquery.ExtensionManagerClient, logger log.Logger) *table.Plugin {
	columns := []table.ColumnDefinition{
		table.TextColumn("path"),
		table.TextColumn("icon"),
	}
	t := &appIconsTable{client: client}
	return table.NewPlugin("kolide_app_icons", columns, t.generateAppIcons)
}

type appIconsTable struct {
	client *osquery.ExtensionManagerClient
	apps   []map[string]string
}

type infoPlist struct {
	CFBundleIconFile string
}

func (t *appIconsTable) generateAppIcons(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	var results []map[string]string
	apps, err := collectApps(t.client)
	if err != nil {
		return nil, errors.Wrap(err, "collecting apps")
	}
	for _, app := range apps {
		img, err := appIcnsToPng(app["path"])
		if err != nil {
			results = append(results, map[string]string{
				"path": app["path"],
				"icon": errors.Wrap(err, "missing").Error(),
			})
			continue
		}
		buf := new(bytes.Buffer)
		img = resize.Resize(128, 128, img, resize.Bilinear)
		png.Encode(buf, img)
		results = append(results, map[string]string{
			"path": app["path"],
			"icon": base64.StdEncoding.EncodeToString(buf.Bytes()),
		})
	}

	return results, nil
}

func appIcnsToPng(appPath string) (image.Image, error) {
	file, err := os.Open(path.Join(appPath, "/Contents/Info.plist"))
	defer file.Close()
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("opening %s Info.plist", appPath))
	}
	var appInfoPlist infoPlist
	if err := plist.NewDecoder(file).Decode(&appInfoPlist); err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("decoding %s Info.plist", appPath))
	}
	pathStr := path.Join(appPath, "Contents/Resources", appInfoPlist.CFBundleIconFile)
	if path.Ext(pathStr) == "" {
		pathStr = pathStr + ".icns"
	}
	if _, err := os.Stat(pathStr); os.IsNotExist(err) {
		return nil, errors.Wrap(err, fmt.Sprintf("icns file doesn't exist"))
	}

	var data C.CFDataRef = 0
	C.Icon(&data, C.CString(pathStr))
	if data == 0 {
		return nil, errors.New("no icon found")
	}
	defer C.CFRelease(C.CFTypeRef(data))
	tiffBytes := C.GoBytes(unsafe.Pointer(C.CFDataGetBytePtr(data)), C.int(C.CFDataGetLength(data)))
	if len(tiffBytes) == 0 {
		return nil, errors.Errorf("no icon image for app %s", appPath)
	}
	img, err := tiff.Decode(bytes.NewBuffer(tiffBytes))
	if err != nil {
		return nil, errors.Wrap(err, "decoding tiff image from C")
	}

	return img, nil
}

func collectApps(client *osquery.ExtensionManagerClient) ([]map[string]string, error) {
	query := `select name, path from apps`
	rows, err := client.QueryRows(query)
	if err != nil {
		return nil, errors.Wrap(err, "querying for apps")
	}
	return rows, nil
}
