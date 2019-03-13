package table

/*
#cgo darwin CFLAGS: -DDARWIN -x objective-c
#cgo darwin LDFLAGS: -framework Cocoa
#include <CoreFoundation/CoreFoundation.h>
#import <Collaboration/Collaboration.h>
void Icon(CFDataRef *iconDataRef, char* path) {
	NSString *appPath = [[NSString alloc] initWithUTF8String:path];
	// nil if path does not identify an accessible bundle directory
	NSBundle *appBundle = [NSBundle bundleWithPath:appPath];
	if (appBundle == nil){
		return;
	}
	NSString *iconFilename = [appBundle objectForInfoDictionaryKey:@"CFBundleIconFile"];
	NSString *iconBasename = [iconFilename stringByDeletingPathExtension];
	NSString *iconExtension = [iconFilename pathExtension];
	NSString *iconPath = [appBundle pathForResource:iconBasename ofType:@"icns"];

	NSImage *img = [[NSImage alloc] initWithContentsOfFile:iconPath];
	//request 512x512 since we are going to resize the icon
	NSRect targetFrame = NSMakeRect(0, 0, 512, 512);
	NSBitmapImageRep *brep = (NSBitmapImageRep *)[img bestRepresentationForRect:targetFrame context:nil hints:nil];
	NSData *imageData = [brep TIFFRepresentation];
	*iconDataRef = (CFDataRef)imageData;
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
	"unsafe"

	"github.com/go-kit/kit/log"
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

func (t *appIconsTable) generateAppIcons(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	var results []map[string]string
	apps, err := collectApps(t.client, queryContext)
	if err != nil {
		return nil, errors.Wrap(err, "collecting installed apps")
	}

	for _, app := range apps {
		img, err := getAppIcon(app["path"])
		if err != nil {
			results = append(results, map[string]string{
				"path": app["path"],
			})
		} else {
			buf := new(bytes.Buffer)
			img = resize.Resize(128, 128, img, resize.Bilinear)
			png.Encode(buf, img)
			results = append(results, map[string]string{
				"path": app["path"],
				"icon": base64.StdEncoding.EncodeToString(buf.Bytes()),
			})
		}
	}

	return results, nil
}

func getAppIcon(appPath string) (image.Image, error) {
	var data C.CFDataRef = 0
	C.Icon(&data, C.CString(appPath))
	if data == 0 {
		return nil, errors.Errorf("no icon image for app %s", appPath)
	}
	defer C.CFRelease(C.CFTypeRef(data))

	tiffBytes := C.GoBytes(unsafe.Pointer(C.CFDataGetBytePtr(data)), C.int(C.CFDataGetLength(data)))
	if len(tiffBytes) == 0 {
		return nil, errors.Errorf("no icon image for app %s", appPath)
	}
	img, err := tiff.Decode(bytes.NewBuffer(tiffBytes))
	if err != nil {
		return nil, errors.Wrap(err, "decoding tiff bytes")
	}

	return img, nil
}

func collectApps(client *osquery.ExtensionManagerClient, queryContext table.QueryContext) ([]map[string]string, error) {
	query := "select name, path from apps"
	if q, ok := queryContext.Constraints["path"]; ok && len(q.Constraints) != 0 {
		query = fmt.Sprintf("%s where path='%s'", query, q.Constraints[0].Expression)
	}
	rows, err := client.QueryRows(query)
	if err != nil {
		return nil, errors.Wrap(err, "querying for apps")
	}
	return rows, nil
}
