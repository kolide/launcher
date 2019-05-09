package packagekit

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"text/template"

	"github.com/google/uuid"
	"github.com/kolide/launcher/pkg/packagekit/authenticode"
	"github.com/kolide/launcher/pkg/packagekit/internal"
	"github.com/kolide/launcher/pkg/packagekit/wix"
	"github.com/pkg/errors"
	"go.opencensus.io/trace"
)

//go:generate go-bindata -nometadata -nocompress -pkg internal -o internal/assets.go internal/assets/

func PackageWixMSI(ctx context.Context, w io.Writer, po *PackageOptions, includeService bool) error {
	ctx, span := trace.StartSpan(ctx, "packagekit.PackageWixMSI")
	defer span.End()

	if err := isDirectory(po.Root); err != nil {
		return err
	}

	// We need to use variables to stub various parts of the wix
	// xml. While we could use wix's internal variable system, it's a
	// little more debugable to do it with go's. This way, we can
	// inspect the intermediate xml file.
	//
	// This might all be cleaner moved from a template to a marshalled
	// struct. But enumerating the wix options looks very ugly
	wixTemplateBytes, err := internal.Asset("internal/assets/main.wxs")
	if err != nil {
		return errors.Wrap(err, "getting go-bindata main.wxs")
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
	}

	wixTemplate, err := template.New("WixTemplate").Parse(string(wixTemplateBytes))
	if err != nil {
		return errors.Wrap(err, "not able to parse main.wxs template")
	}

	mainWxsContent := new(bytes.Buffer)
	if err := wixTemplate.ExecuteTemplate(mainWxsContent, "WixTemplate", templateData); err != nil {
		return errors.Wrap(err, "executing WixTemplate")
	}

	wixArgs := []wix.WixOpt{}

	if includeService {
		launcherService := wix.NewService("launcher.exe",
			wix.ServiceName(fmt.Sprintf("Launcher%sSvc", strings.Title(po.Identifier))),
			wix.ServiceArgs([]string{"svc", "-config", po.FlagFile}),
			wix.ServiceDescription(fmt.Sprintf("The Kolide Launcher (%s)", po.Identifier)),
		)
		wixArgs = append(wixArgs, wix.WithService(launcherService))
	}

	wixTool, err := wix.New(po.Root, mainWxsContent.Bytes(), wixArgs...)
	if err != nil {
		return errors.Wrap(err, "making wixTool")
	}
	defer wixTool.Cleanup()

	// Use wix to compile into an MSI
	msiFile, err := wixTool.Package(ctx)
	if err != nil {
		return errors.Wrap(err, "wix packaging")
	}

	// Sign?
	if po.SigningKey != "" {
		if err := authenticode.Sign(ctx, msiFile, authenticode.WithSubjectName(po.SigningKey)); err != nil {
			return errors.Wrap(err, "authencode signing")
		}
	}

	// Copy MSI into our filehandle
	msiFH, err := os.Open(msiFile)
	if err != nil {
		return errors.Wrap(err, "opening msi output file")
	}
	defer msiFH.Close()

	if _, err := io.Copy(w, msiFH); err != nil {
		return errors.Wrap(err, "copying output")
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
