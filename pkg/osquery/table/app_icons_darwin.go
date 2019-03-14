package table

/*
#cgo darwin CFLAGS: -DDARWIN -x objective-c
#cgo darwin LDFLAGS: -framework Cocoa
#import <Appkit/AppKit.h>
void Icon(CFDataRef *iconDataRef, char* path) {
	NSString *appPath = [[NSString stringWithUTF8String:path] stringByStandardizingPath];
	NSImage *img = [[NSWorkspace sharedWorkspace] iconForFile:appPath];

	//request 512x512 since we are going to resize the icon
	NSRect targetFrame = NSMakeRect(0, 0, 128, 128);
	CGImageRef cgref = [img CGImageForProposedRect:&targetFrame context:nil hints:nil];
	NSBitmapImageRep *brep = [[NSBitmapImageRep alloc] initWithCGImage:cgref];
	NSData *imageData = [brep TIFFRepresentation];
	*iconDataRef = (CFDataRef)imageData;
}
*/
import "C"
import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/png"
	"unsafe"

	"github.com/kolide/osquery-go/plugin/table"
	"github.com/nfnt/resize"
	"github.com/pkg/errors"
	"golang.org/x/image/tiff"
)

func AppIcons() *table.Plugin {
	columns := []table.ColumnDefinition{
		table.TextColumn("path"),
		table.TextColumn("icon"),
	}
	return table.NewPlugin("kolide_app_icons", columns, generateAppIcons)
}

func generateAppIcons(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	q, ok := queryContext.Constraints["path"]
	if !ok || len(q.Constraints) == 0 {
		return nil, errors.New("The kolide_app_icons table requires that you specify a constraint WHERE path =")
	}
	path := q.Constraints[0].Expression
	img, err := getAppIcon(path)
	if err != nil {
		return nil, err
	}

	var results []map[string]string
	buf := new(bytes.Buffer)
	img = resize.Resize(128, 128, img, resize.Bilinear)
	png.Encode(buf, img)
	results = append(results, map[string]string{
		"path": path,
		"icon": base64.StdEncoding.EncodeToString(buf.Bytes()),
	})

	return results, nil
}

func getAppIcon(appPath string) (image.Image, error) {
	var data C.CFDataRef = 0
	C.Icon(&data, C.CString(appPath))
	defer C.CFRelease(C.CFTypeRef(data))

	tiffBytes := C.GoBytes(unsafe.Pointer(C.CFDataGetBytePtr(data)), C.int(C.CFDataGetLength(data)))
	img, err := tiff.Decode(bytes.NewBuffer(tiffBytes))
	if err != nil {
		return nil, errors.Wrap(err, "decoding tiff bytes")
	}

	return img, nil
}
