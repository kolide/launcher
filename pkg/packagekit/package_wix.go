package packagekit

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"text/template"

	"github.com/google/uuid"
	"github.com/kolide/kit/version"
	"github.com/kolide/launcher/pkg/packagekit/authenticode"
	"github.com/kolide/launcher/pkg/packagekit/wix"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// We need to use variables to stub various parts of the wix
// xml. While we could use wix's internal variable system, it's a
// little more debugable to do it with go's. This way, we can
// inspect the intermediate xml file.
//
// This might all be cleaner moved from a template to a marshalled
// struct. But enumerating the wix options looks very ugly
//
//go:embed assets/main.wxs
var wixTemplateBytes []byte

// This is used for icons and splash screens and the like. It would be
// better in pkg/packaging, and passed into packagekit, but that's a
// deeper refactor.
//
//go:embed assets/*
var assets embed.FS

var signtoolVersionRegex = regexp.MustCompile(`^(.+)\/x64\/signtool\.exe$`)

func PackageWixMSI(ctx context.Context, w io.Writer, po *PackageOptions, includeService bool) error {
	if err := isDirectory(po.Root); err != nil {
		return err
	}

	// populate VersionNum if it isn't already set by the caller. we'll
	// store this in the registry on install to give a comparable field
	// for intune to drive upgrade behavior from
	if po.VersionNum == 0 {
		po.VersionNum = version.VersionNumFromSemver(po.Version)
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
		return fmt.Errorf("generating uuid as guid nonce: %w", err)

	}
	extraGuidIdentifiers := []string{
		po.Version,
		runtime.GOARCH,
		guidNonce.String(),
	}

	var templateData = struct {
		Opts            *PackageOptions
		UpgradeCode     string
		ProductCode     string
		PermissionsGUID string
	}{
		Opts:        po,
		UpgradeCode: generateMicrosoftProductCode("launcher" + po.Identifier),
		ProductCode: generateMicrosoftProductCode("launcher"+po.Identifier, extraGuidIdentifiers...),
		// our permissions component does not meet the criteria to have it's GUID automatically generated - but we should
		// ensure it is unique for each build so we regenerate here alongside the product and upgrade codes
		PermissionsGUID: generateMicrosoftProductCode("launcher_root_dir_permissions"+po.Identifier, extraGuidIdentifiers...),
	}

	wixTemplate, err := template.New("WixTemplate").Parse(string(wixTemplateBytes))
	if err != nil {
		return fmt.Errorf("not able to parse main.wxs template: %w", err)
	}

	mainWxsContent := new(bytes.Buffer)
	if err := wixTemplate.ExecuteTemplate(mainWxsContent, "WixTemplate", templateData); err != nil {
		return fmt.Errorf("executing WixTemplate: %w", err)
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
			fileBytes, err := assets.ReadFile(path.Join("assets", f))
			if err != nil {
				return fmt.Errorf("getting asset %s: %w", f, err)
			}

			wixArgs = append(wixArgs, wix.WithFile(f, fileBytes))
		}
	}

	if includeService {
		launcherService := wix.NewService("launcher.exe",
			// Ensure that the service does not start until DNS is available, to avoid unrecoverable DNS failures in launcher.
			wix.WithServiceDependency("Dnscache"),
			wix.ServiceName(fmt.Sprintf("Launcher%sSvc", cases.Title(language.Und, cases.NoLower).String(po.Identifier))),
			wix.ServiceArgs([]string{"svc", "-config", po.FlagFile}),
			wix.ServiceDescription(fmt.Sprintf("The Kolide Launcher (%s)", po.Identifier)),
		)

		if po.DisableService {
			wix.WithDisabledService()(launcherService)
		}

		wixArgs = append(wixArgs, wix.WithService(launcherService))
	}

	wixTool, err := wix.New(po.Root, po.Identifier, mainWxsContent.Bytes(), wixArgs...)
	if err != nil {
		return fmt.Errorf("making wixTool: %w", err)
	}
	defer wixTool.Cleanup()

	// Use wix to compile into an MSI
	msiFile, err := wixTool.Package(ctx)
	if err != nil {
		return fmt.Errorf("wix packaging: %w", err)
	}

	// Sign?
	if po.WindowsUseSigntool {
		signtoolPath, err := getSigntoolPath()
		if err != nil {
			return fmt.Errorf("looking up signtool location: %w", err)
		}
		if err := authenticode.Sign(
			ctx, msiFile,
			authenticode.WithExtraArgs(po.WindowsSigntoolArgs),
			authenticode.WithSigntoolPath(signtoolPath),
		); err != nil {
			return fmt.Errorf("authenticode signing: %w", err)
		}
	}

	// Copy MSI into our filehandle
	msiFH, err := os.Open(msiFile)
	if err != nil {
		return fmt.Errorf("opening msi output file: %w", err)
	}
	defer msiFH.Close()

	if _, err := io.Copy(w, msiFH); err != nil {
		return fmt.Errorf("copying output: %w", err)
	}

	SetInContext(ctx, ContextLauncherVersionKey, po.Version)

	return nil
}

// getSigntoolPath attempts to look up the location of signtool so that
// we do not have to rely on a hard-coded signtool location that will change
// when we upgrade to a new version of signtool.
func getSigntoolPath() (string, error) {
	var signtoolPath string
	signtoolVersion := "0.0.0.0"

	root := `C:\Program Files (x86)\Windows Kits\10\bin` // restrict our lookup to a well-known location
	fileSystem := os.DirFS(root)

	if err := fs.WalkDir(fileSystem, ".", func(currentPath string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// We expect signing to happen on a build machine with 64-bit architecture, so we restrict
		// our matches accordingly
		if !d.IsDir() && strings.HasSuffix(currentPath, `x64/signtool.exe`) {
			// Parse out the version -- we expect the current path to look something like 10.0.18362.0/x64/signtool.exe
			versionMatches := signtoolVersionRegex.FindStringSubmatch(currentPath)
			if len(versionMatches) < 2 {
				return nil
			}

			// We can't parse it as a semver, but simple string comparison works fine.
			if versionMatches[1] > signtoolVersion {
				signtoolVersion = versionMatches[1]
				signtoolPath = filepath.Join(root, currentPath)
			}

			return nil
		}
		return nil
	}); err != nil {
		return "", fmt.Errorf("walking %s: %w", root, err)
	}

	if signtoolPath == "" {
		return "", fmt.Errorf("signtool.exe not found in %s", root)
	}

	return signtoolPath, nil
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
