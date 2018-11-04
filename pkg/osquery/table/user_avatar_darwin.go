package table

/*
#cgo darwin CFLAGS: -DDARWIN -x objective-c
#cgo darwin LDFLAGS: -framework Cocoa -framework Collaboration
#import <Collaboration/Collaboration.h>
#include <CoreFoundation/CoreFoundation.h>
void Image(CFDataRef *imageDataRef, char* user) {
NSString *userName = [NSString stringWithFormat:@"%s", user];
CBIdentity *identity = [CBIdentity identityWithName:userName authority:[CBIdentityAuthority defaultIdentityAuthority]];
NSImage *userImage = [identity image];
NSData *imageData = [userImage TIFFRepresentation];
*imageDataRef = (CFDataRef)imageData;
}
*/
import "C"
import (
	"bytes"
	"context"
	"encoding/base64"
	"image/png"
	"unsafe"

	osquery "github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"
	"github.com/nfnt/resize"
	"github.com/pkg/errors"
	"golang.org/x/image/tiff"
)

func UserAvatar(client *osquery.ExtensionManagerClient) *table.Plugin {
	columns := []table.ColumnDefinition{
		table.TextColumn("username"),
		table.TextColumn("thumbnail"),
	}
	t := &userAvatarTable{client: client}
	return table.NewPlugin("kolide_user_avatar", columns, t.generateAvatar)
}

type userAvatarTable struct {
	client      *osquery.ExtensionManagerClient
	primaryUser string
}

func (t *userAvatarTable) generateAvatar(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {

	if t.primaryUser == "" {
		username, err := queryPrimaryUser(t.client)
		if err != nil {
			return nil, errors.Wrap(err, "query primary user for airdrop table")
		}
		t.primaryUser = username
	}

	// use the username from the query context if provide, otherwise default to primary user
	var username string
	q, ok := queryContext.Constraints["username"]
	if ok && len(q.Constraints) != 0 {
		username = q.Constraints[0].Expression
	} else {
		username = t.primaryUser
	}

	var data C.CFDataRef = 0
	C.Image(&data, C.CString(username))
	defer C.CFRelease(C.CFTypeRef(data))
	goBytes := C.GoBytes(unsafe.Pointer(C.CFDataGetBytePtr(data)), C.int(C.CFDataGetLength(data)))

	image, err := tiff.Decode(bytes.NewBuffer(goBytes))
	if err != nil {
		return nil, errors.Wrap(err, "decoding tiff image from C")
	}

	var base64Buf bytes.Buffer
	encoder := base64.NewEncoder(base64.StdEncoding, &base64Buf)
	defer encoder.Close()

	thumbnail := resize.Thumbnail(150, 150, image, resize.Lanczos3)

	if err := png.Encode(encoder, thumbnail); err != nil {
		return nil, errors.Wrap(err, "encode resized user avatar to png")
	}

	return []map[string]string{
		map[string]string{
			"username":  username,
			"thumbnail": base64Buf.String(),
		},
	}, nil
}
