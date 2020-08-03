package packagekit

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"text/template"

	"github.com/go-kit/kit/log/level"
	"github.com/google/uuid"
	"github.com/kolide/launcher/pkg/contexts/ctxlog"
	"github.com/kolide/launcher/pkg/packagekit/authenticode"
	"github.com/kolide/launcher/pkg/packagekit/internal"
	"github.com/kolide/launcher/pkg/packagekit/wix"
	"github.com/pkg/errors"
	"go.opencensus.io/trace"
)

//go:generate go-bindata -nometadata -nocompress -pkg internal -o internal/assets.go internal/assets/

const (
	signtoolPath = `C:\Program Files (x86)\Windows Kits\10\bin\10.0.18362.0\x64\signtool.exe`
)

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

	// We include a random nonce as part of the ProductCode
	// guid. This is so that any MSI rebuild triggers the Major
	// Upgrade flow, and not the "Another version of this product
	// is already installed" error. The Minor Upgrade Flow might
	// be more appropriate, but requires substantial reworking of
	// how versions and builds are calculated. See
	// https://www.firegiant.com/wix/tutorial/upgrades-and-modularization/
	// for opinionated background
	guidNonce, err := uuid.NewRandom()
	if err != nil {
		return errors.Wrap(err, "generating uuid as guid nonce")

	}
	extraGuidIdentifiers := []string{
		po.Version,
		runtime.GOARCH,
		guidNonce.String(),
	}

	// Fetch postinstall info
	postinstCmd, err := generatePostinstallCommand(ctx, po)
	if err != nil {
		return errors.Wrap(err, "generate postinstall command")
	}

	var templateData = struct {
		Opts           *PackageOptions
		UpgradeCode    string
		ProductCode    string
		PostInstallCmd string
	}{
		Opts:           po,
		UpgradeCode:    generateMicrosoftProductCode("launcher" + po.Identifier),
		ProductCode:    generateMicrosoftProductCode("launcher"+po.Identifier, extraGuidIdentifiers...),
		PostInstallCmd: postinstCmd,
	}

	wixTemplate, err := template.New("WixTemplate").Parse(string(wixTemplateBytes))
	if err != nil {
		return errors.Wrap(err, "parsing main.wxs template")
	}

	mainWxsContent := new(bytes.Buffer)
	if err := wixTemplate.ExecuteTemplate(mainWxsContent, "WixTemplate", templateData); err != nil {
		return errors.Wrap(err, "executing WixTemplate")
	}

	wixArgs := []wix.WixOpt{}

	if po.WixSkipCleanup {
		wixArgs = append(wixArgs, wix.SkipCleanup())
	}

	if po.WixPath != "" {
		wixArgs = append(wixArgs, wix.WithWix(po.WixPath))
	}

	{
		// Regardless of whether or not there's a UI in the MSI, we
		// still want the icon file to be included.
		assetFiles := []string{"kolide.ico"}

		if po.WixUI {
			assetFiles = append(assetFiles, "msi_banner.bmp", "msi_splash.bmp")
			wixArgs = append(wixArgs, wix.WithUI())
		}

		for _, f := range assetFiles {
			fileBytes, err := internal.Asset("internal/assets/" + f)
			if err != nil {
				return errors.Wrapf(err, "getting go-bindata %s", f)
			}

			wixArgs = append(wixArgs, wix.WithFile(f, fileBytes))
		}
	}

	if includeService {
		launcherService := wix.NewService("launcher.exe",
			wix.WithDelayedStart(),
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
	if po.WindowsUseSigntool {
		if err := authenticode.Sign(
			ctx, msiFile,
			authenticode.WithExtraArgs(po.WindowsSigntoolArgs),
			authenticode.WithSigntoolPath(signtoolPath),
		); err != nil {
			return errors.Wrap(err, "authenticode signing")
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

// generatePostinstall creates a postinstall block for our wix
// template. To make this work with the existing package/packagekit
// split, this makes a lot of assumptions about paths. Other methods
// would involve pulling the File.Id out of the heat generated
// AppFiles.
func generatePostinstallCommand(ctx context.Context, po *PackageOptions) (string, error) {
	launcherPath, err := findFileInDir(ctx, po.Root, "launcher.exe")
	if err != nil {
		return "", err
	}

	confPath, err := findFileInDir(ctx, po.Root, "launcher.flags")
	if err != nil {
		return "", err
	}

	// A big string, of quoted strings. Joy
	return strings.Join([]string{
		fmt.Sprintf(`"%s"`, filepath.Clean(filepath.Join("[PROGDIR]", launcherPath))),
		"postinstall",
		"--installer_path", `"[OriginalDatabase]"`,
		"--config", confPath,
		"--identifier", po.Identifier,
	}, " "), nil
}

func findFileInDir(ctx context.Context, dir, filename string) (string, error) {
	logger := ctxlog.FromContext(ctx)

	dir = filepath.Clean(dir)

	level.Debug(logger).Log(
		"msg", "Looking for file",
		"file", filename,
		"dir", dir,
	)

	found := []string{}

	if err := filepath.Walk(dir, func(path string, f os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if filepath.Base(path) == filename {
			found = append(found, path)
		}
		return nil
	}); err != nil {
		return "", errors.Wrap(err, "walk")
	}

	if len(found) != 1 {
		return "", errors.Errorf("Found %d %s, expected 1", len(found), filename)
	}

	rel := strings.TrimPrefix(filepath.Clean(found[0]), dir)
	rel = strings.TrimPrefix(rel, `\`)
	return rel, nil
}
