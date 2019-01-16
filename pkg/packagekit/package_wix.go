package packagekit

import (
	"bytes"
	"context"
	"io"
	"io/ioutil"
	"os"
	"runtime"
	"strings"
	"text/template"

	"github.com/google/uuid"
	"github.com/kolide/launcher/pkg/packagekit/internal"
	"github.com/kolide/launcher/pkg/packagekit/wix"
	"github.com/pkg/errors"
	"go.opencensus.io/trace"
)

//go:generate go-bindata   -nocompress -pkg internal -o internal/assets.go internal/assets/

func PackageWixMSI(ctx context.Context, w io.Writer, po *PackageOptions) error {
	ctx, span := trace.StartSpan(ctx, "packagekit.PackageWixMSI")
	defer span.End()

	if err := isDirectory(po.Root); err != nil {
		return err
	}

	buildDir, err := ioutil.TempDir("", "wix-build")
	if err != nil {
		return errors.Wrap(err, "making wix build dir")
	}
	defer os.RemoveAll(buildDir)

	wixTool, err := wix.New(po.Root, buildDir)
	if err != nil {
		return errors.Wrap(err, "making wixTool")
	}

	// We need to use variables to stub various parts of the wix
	// xml. While we could use wix's internal variable system, it's a
	// little more debugable to do it with go's. This way, we can
	// inspect the intermediate xml file.
	//
	// This might all be cleaner moved to a marshalled struct. For now,
	// just sent the template the PackageOptions struct
	wixTemplateBytes, err := internal.Asset("internal/assets/installer.wxs")
	if err != nil {
		return errors.Wrap(err, "getting go-bindata install.wxs")
	}

	extraGuidIdentifiers := []string{
		po.Version,
		runtime.GOARCH,
	}

	var templateData = struct {
		Opts        *PackageOptions
		UpgradeCode string
		ProductCode string
		PackageCode string
	}{
		Opts:        po,
		UpgradeCode: generateMicrosoftProductCode("launcher" + po.Identifier),
		ProductCode: generateMicrosoftProductCode("launcher"+po.Identifier, extraGuidIdentifiers...),
		PackageCode: generateMicrosoftProductCode("launcher"+po.Identifier, extraGuidIdentifiers...),
	}

	wixTemplate, err := template.New("WixTemplate").Parse(string(wixTemplateBytes))
	if err != nil {
		return errors.Wrap(err, "not able to parse Install.wxs template")
	}

	installWXS := new(bytes.Buffer)
	if err := wixTemplate.ExecuteTemplate(installWXS, "WixTemplate", templateData); err != nil {
		return errors.Wrap(err, "executing WixTemplate")
	}

	if err := wixTool.InstallWXS(installWXS.Bytes()); err != nil {
		return errors.Wrap(err, "installing WixTemplate")
	}

	if err := wixTool.Heat(ctx); err != nil {
		return errors.Wrap(err, "running heat")
	}

	if err := wixTool.Candle(ctx); err != nil {
		return errors.Wrap(err, "running candle")
	}

	// Run light to compile the msi (and copy the output into our file
	// handle)
	if err := wixTool.Light(ctx, w); err != nil {
		return errors.Wrap(err, "running light")
	}

	return nil
}

// generateMicrosoftProductCode create a stable guid from a set of
// inputs. This is used to identify the product / sub product /
// package / version, and whatnot. We need to either store them, or
// generate them in a predictable fasion based on a set of inputs. See
// doc.go, or
// https://docs.microsoft.com/en-us/windows/desktop/Msi/productcode
//
// It is equivlent to uuid.NewSHA1(kolideUuidSpace,
// []byte(launcherkolide-app0.7.0amd64)) but provided here so we have
// a clear point to test stability against.
func generateMicrosoftProductCode(ident1 string, identN ...string) string {
	// Define a Kolide uuid space. This could also have used uuid.NameSpaceDNS
	uuidSpace := uuid.NewSHA1(uuid.Nil, []byte("Kolide"))

	data := strings.Join(append([]string{ident1}, identN...), "")

	guid := uuid.NewSHA1(uuidSpace, []byte(data))

	return strings.ToUpper(guid.String())
}
