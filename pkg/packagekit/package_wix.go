package packagekit

import (
	"bytes"
	"context"
	"crypto/md5"
	"fmt"
	"io"
	"io/ioutil"
	"runtime"
	"text/template"

	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/contexts/ctxlog"
	"github.com/kolide/launcher/pkg/packagekit/internal"
	"github.com/kolide/launcher/pkg/packagekit/wix"
	"github.com/pkg/errors"
	"go.opencensus.io/trace"
)

func GimmieWXS() ([]byte, error) {
	return internal.InstallWXS()
}

func PackageWixMSI(ctx context.Context, w io.Writer, po *PackageOptions) error {
	ctx, span := trace.StartSpan(ctx, "packagekit.PackageWixMSI")
	defer span.End()

	logger := ctxlog.FromContext(ctx)

	if err := isDirectory(po.Root); err != nil {
		return err
	}

	buildDir, err := ioutil.TempDir("", "wix-build")
	if err != nil {
		panic(err)
	}
	level.Debug(logger).Log("builddir", buildDir)
	// TODO remove this on cleanup

	wixTool, err := wix.New(po.Root, buildDir)
	if err != nil {
		panic(err)
	}

	// We need to use variables to stub various parts of the wix
	// xml. While we could use wix's internal variable system, it's a
	// little more debugable to do it with go's. This way, we can
	// inspect the intermediate xml file.
	//
	// This might all be cleaner moved to a marshalled struct. For now,
	// just sent the template the PackageOptions struct
	wixTemplateBytes, err := internal.Asset("assets/installer.wxs")
	if err != nil {
		return errors.Wrap(err, "getting go-bindata install.wxs")
	}

	extraGuidIdentifiers := []string{
		runtime.GOARCH,
		po.Version,
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

// generateMicrosoftProductCode is a stable guid that is used to
// identify the product / sub product / package / version, and
// whatnot. We need to either store them, or generate them in a
// predictable fasion based on a set of inputs. See doc.go, or
// https://docs.microsoft.com/en-us/windows/desktop/Msi/productcode
func generateMicrosoftProductCode(ident1 string, identN ...string) string {
	h := md5.New()
	io.WriteString(h, ident1)
	for _, s := range identN {
		io.WriteString(h, s)
	}

	hash := h.Sum(nil)

	return fmt.Sprintf("%X-%X-%X-%X-%X", hash[0:4], hash[4:6], hash[6:8], hash[8:10], hash[10:16])
}
