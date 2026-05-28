package windowsupdate

import (
	"fmt"
	"time"

	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
)

// IUpdate contains the properties and methods that are available to an update.
// https://docs.microsoft.com/en-us/windows/win32/api/wuapi/nn-wuapi-iupdate
type IUpdate struct {
	disp                            *ole.IDispatch
	AutoDownload                    int32 // enum https://docs.microsoft.com/en-us/windows/win32/api/wuapi/nf-wuapi-iupdate5-get_autodownload
	AutoSelection                   int32 // enum https://docs.microsoft.com/en-us/windows/win32/api/wuapi/nf-wuapi-iupdate5-get_autoselection
	AutoSelectOnWebSites            bool
	BundledUpdates                  []*IUpdateIdentity // These are full IUpdate objects, but we truncate them
	BrowseOnly                      bool               // From IUpdate3
	CanRequireSource                bool
	Categories                      []*ICategory
	CveIDs                          []string // From IUpdate2
	Deadline                        *time.Time
	DeltaCompressedContentAvailable bool
	DeltaCompressedContentPreferred bool
	DeploymentAction                int32 // enum https://docs.microsoft.com/en-us/windows/win32/api/wuapi/ne-wuapi-deploymentaction
	Description                     string
	DownloadContents                []*IUpdateDownloadContent
	DownloadPriority                int32 // enum https://docs.microsoft.com/en-us/windows/win32/api/wuapi/ne-wuapi-downloadpriority
	EulaAccepted                    bool
	EulaText                        string
	HandlerID                       string
	Identity                        *IUpdateIdentity
	Image                           *IImageInformation
	InstallationBehavior            *IInstallationBehavior
	IsBeta                          bool
	IsDownloaded                    bool
	IsHidden                        bool
	IsInstalled                     bool
	IsMandatory                     bool
	IsPresent                       bool // From IUpdate2
	IsUninstallable                 bool
	KBArticleIDs                    []string
	Languages                       []string
	LastDeploymentChangeTime        *time.Time
	MaxDownloadSize                 int64
	MinDownloadSize                 int64
	MoreInfoUrls                    []string
	MsrcSeverity                    string
	PerUser                         bool // From IUpdate4
	RebootRequired                  bool // From IUpdate2
	RecommendedCpuSpeed             int32
	RecommendedHardDiskSpace        int32
	RecommendedMemory               int32
	ReleaseNotes                    string
	SecurityBulletinIDs             []string
	SupersededUpdateIDs             []string
	SupportUrl                      string
	Title                           string
	UninstallationBehavior          *IInstallationBehavior
	UninstallationNotes             string
	UninstallationSteps             []string
}

// toIUpdates takes a IUpdateCollection and returns a []*IUpdate
func toIUpdates(updatesDisp *ole.IDispatch) ([]*IUpdate, error) {
	count, err := getPropertyInt32(updatesDisp, "Count")
	if err != nil {
		return nil, fmt.Errorf("Count: %w", err)
	}

	updates := make([]*IUpdate, count)
	for i := 0; i < int(count); i++ {
		updateDisp, err := getPropertyDispatch(updatesDisp, "Item", i)
		if err != nil {
			return nil, fmt.Errorf("Item[%d/%d]: %w", i, count, err)
		}

		update, err := toIUpdate(updateDisp)
		if err != nil {
			return nil, fmt.Errorf("converting Item[%d/%d]: %w", i, count, err)
		}

		updates[i] = update
	}
	return updates, nil
}

// toIUpdatesIdentities takes a IUpdateCollection and returns the
// []*IUpdateIdentity of the contained IUpdates. This is *not* recursive, though possibly should be.
func toIUpdatesIdentities(updatesDisp *ole.IDispatch) ([]*IUpdateIdentity, error) {
	if updatesDisp == nil {
		return nil, nil
	}

	count, err := getPropertyInt32(updatesDisp, "Count")
	if err != nil {
		return nil, fmt.Errorf("Count: %w", err)
	}

	identities := make([]*IUpdateIdentity, count)
	for i := 0; i < int(count); i++ {
		id, err := extractUpdateIdentity(updatesDisp, i, int(count))
		if err != nil {
			return nil, err
		}
		identities[i] = id
	}
	return identities, nil
}

func extractUpdateIdentity(updatesDisp *ole.IDispatch, i, count int) (*IUpdateIdentity, error) {
	updateDisp, err := getPropertyDispatch(updatesDisp, "Item", i)
	if err != nil {
		return nil, fmt.Errorf("Item[%d/%d]: %w", i, count, err)
	}
	defer updateDisp.Release()

	identityDisp, err := getPropertyDispatch(updateDisp, "Identity")
	if err != nil {
		return nil, fmt.Errorf("Identity[%d/%d]: %w", i, count, err)
	}
	if identityDisp == nil {
		return nil, nil
	}
	id, err := toIUpdateIdentity(identityDisp)
	if err != nil {
		return nil, fmt.Errorf("converting Identity[%d/%d]: %w", i, count, err)
	}
	return id, nil
}

func toIUpdate(updateDisp *ole.IDispatch) (*IUpdate, error) {
	// We keep the disp alive for AcceptEula() and other methods that need it.
	// Callers that don't need it can call Release() when done.
	var err error
	iUpdate := &IUpdate{
		disp: updateDisp,
	}

	if iUpdate.AutoDownload, err = getPropertyInt32(updateDisp, "AutoDownload"); err != nil {
		return nil, err
	}

	if iUpdate.AutoSelection, err = getPropertyInt32(updateDisp, "AutoSelection"); err != nil {
		return nil, err
	}

	if iUpdate.AutoSelectOnWebSites, err = getPropertyBool(updateDisp, "AutoSelectOnWebSites"); err != nil {
		return nil, err
	}

	if arrDisp, err := getPropertyDispatch(updateDisp, "BundledUpdates"); err != nil {
		return nil, err
	} else if arrDisp != nil {
		defer arrDisp.Release()
		if iUpdate.BundledUpdates, err = toIUpdatesIdentities(arrDisp); err != nil {
			return nil, err
		}
	}

	if iUpdate.BrowseOnly, err = getPropertyBool(updateDisp, "BrowseOnly"); err != nil {
		return nil, err
	}

	if iUpdate.CanRequireSource, err = getPropertyBool(updateDisp, "CanRequireSource"); err != nil {
		return nil, err
	}

	if categoriesDisp, err := getPropertyDispatch(updateDisp, "Categories"); err != nil {
		return nil, err
	} else if categoriesDisp != nil {
		defer categoriesDisp.Release()
		if iUpdate.Categories, err = toICategories(categoriesDisp); err != nil {
			return nil, err
		}
	}

	if cveDisp, err := getPropertyDispatch(updateDisp, "CveIDs"); err != nil {
		return nil, err
	} else if cveDisp != nil {
		defer cveDisp.Release()
		if iUpdate.CveIDs, err = iStringCollectionToStringArray(cveDisp); err != nil {
			return nil, err
		}
	}

	if iUpdate.Deadline, err = getPropertyTime(updateDisp, "Deadline"); err != nil {
		return nil, err
	}

	if iUpdate.DeltaCompressedContentAvailable, err = getPropertyBool(updateDisp, "DeltaCompressedContentAvailable"); err != nil {
		return nil, err
	}

	if iUpdate.DeltaCompressedContentPreferred, err = getPropertyBool(updateDisp, "DeltaCompressedContentPreferred"); err != nil {
		return nil, err
	}

	if iUpdate.DeploymentAction, err = getPropertyInt32(updateDisp, "DeploymentAction"); err != nil {
		return nil, err
	}

	if iUpdate.Description, err = getPropertyString(updateDisp, "Description"); err != nil {
		return nil, err
	}

	if downloadContentsDisp, err := getPropertyDispatch(updateDisp, "DownloadContents"); err != nil {
		return nil, err
	} else if downloadContentsDisp != nil {
		defer downloadContentsDisp.Release()
		if iUpdate.DownloadContents, err = toIUpdateDownloadContents(downloadContentsDisp); err != nil {
			return nil, err
		}
	}

	if iUpdate.DownloadPriority, err = getPropertyInt32(updateDisp, "DownloadPriority"); err != nil {
		return nil, err
	}

	if iUpdate.EulaAccepted, err = getPropertyBool(updateDisp, "EulaAccepted"); err != nil {
		return nil, err
	}

	if iUpdate.EulaText, err = getPropertyString(updateDisp, "EulaText"); err != nil {
		return nil, err
	}

	if iUpdate.HandlerID, err = getPropertyString(updateDisp, "HandlerID"); err != nil {
		return nil, err
	}

	if identityDisp, err := getPropertyDispatch(updateDisp, "Identity"); err != nil {
		return nil, err
	} else if identityDisp != nil {
		// toIUpdateIdentity calls Release() on identityDisp internally
		if iUpdate.Identity, err = toIUpdateIdentity(identityDisp); err != nil {
			return nil, err
		}
	}

	if imageDisp, err := getPropertyDispatch(updateDisp, "Image"); err != nil {
		return nil, err
	} else if imageDisp != nil {
		defer imageDisp.Release()
		if iUpdate.Image, err = toIImageInformation(imageDisp); err != nil {
			return nil, err
		}
	}

	if installBehaviorDisp, err := getPropertyDispatch(updateDisp, "InstallationBehavior"); err != nil {
		return nil, err
	} else if installBehaviorDisp != nil {
		defer installBehaviorDisp.Release()
		if iUpdate.InstallationBehavior, err = toIInstallationBehavior(installBehaviorDisp); err != nil {
			return nil, err
		}
	}

	if iUpdate.IsBeta, err = getPropertyBool(updateDisp, "IsBeta"); err != nil {
		return nil, err
	}

	if iUpdate.IsDownloaded, err = getPropertyBool(updateDisp, "IsDownloaded"); err != nil {
		return nil, err
	}

	if iUpdate.IsHidden, err = getPropertyBool(updateDisp, "IsHidden"); err != nil {
		return nil, err
	}

	if iUpdate.IsInstalled, err = getPropertyBool(updateDisp, "IsInstalled"); err != nil {
		return nil, err
	}

	if iUpdate.IsMandatory, err = getPropertyBool(updateDisp, "IsMandatory"); err != nil {
		return nil, err
	}

	if iUpdate.IsPresent, err = getPropertyBool(updateDisp, "IsPresent"); err != nil {
		return nil, err
	}

	if iUpdate.IsUninstallable, err = getPropertyBool(updateDisp, "IsUninstallable"); err != nil {
		return nil, err
	}

	if kbDisp, err := getPropertyDispatch(updateDisp, "KBArticleIDs"); err != nil {
		return nil, err
	} else if kbDisp != nil {
		defer kbDisp.Release()
		if iUpdate.KBArticleIDs, err = iStringCollectionToStringArray(kbDisp); err != nil {
			return nil, err
		}
	}

	if langDisp, err := getPropertyDispatch(updateDisp, "Languages"); err != nil {
		return nil, err
	} else if langDisp != nil {
		defer langDisp.Release()
		if iUpdate.Languages, err = iStringCollectionToStringArray(langDisp); err != nil {
			return nil, err
		}
	}

	if iUpdate.LastDeploymentChangeTime, err = getPropertyTime(updateDisp, "LastDeploymentChangeTime"); err != nil {
		return nil, err
	}

	if iUpdate.MaxDownloadSize, err = getPropertyInt64(updateDisp, "MaxDownloadSize"); err != nil {
		return nil, err
	}

	if iUpdate.MinDownloadSize, err = getPropertyInt64(updateDisp, "MinDownloadSize"); err != nil {
		return nil, err
	}

	if moreInfoDisp, err := getPropertyDispatch(updateDisp, "MoreInfoUrls"); err != nil {
		return nil, err
	} else if moreInfoDisp != nil {
		defer moreInfoDisp.Release()
		if iUpdate.MoreInfoUrls, err = iStringCollectionToStringArray(moreInfoDisp); err != nil {
			return nil, err
		}
	}

	if iUpdate.MsrcSeverity, err = getPropertyString(updateDisp, "MsrcSeverity"); err != nil {
		return nil, err
	}

	if iUpdate.PerUser, err = getPropertyBool(updateDisp, "PerUser"); err != nil {
		return nil, err
	}

	if iUpdate.RebootRequired, err = getPropertyBool(updateDisp, "RebootRequired"); err != nil {
		return nil, err
	}

	if iUpdate.RecommendedCpuSpeed, err = getPropertyInt32(updateDisp, "RecommendedCpuSpeed"); err != nil {
		return nil, err
	}

	if iUpdate.RecommendedHardDiskSpace, err = getPropertyInt32(updateDisp, "RecommendedHardDiskSpace"); err != nil {
		return nil, err
	}

	if iUpdate.RecommendedMemory, err = getPropertyInt32(updateDisp, "RecommendedMemory"); err != nil {
		return nil, err
	}

	if iUpdate.ReleaseNotes, err = getPropertyString(updateDisp, "ReleaseNotes"); err != nil {
		return nil, err
	}

	if secBulletinDisp, err := getPropertyDispatch(updateDisp, "SecurityBulletinIDs"); err != nil {
		return nil, err
	} else if secBulletinDisp != nil {
		defer secBulletinDisp.Release()
		if iUpdate.SecurityBulletinIDs, err = iStringCollectionToStringArray(secBulletinDisp); err != nil {
			return nil, err
		}
	}

	if supersededDisp, err := getPropertyDispatch(updateDisp, "SupersededUpdateIDs"); err != nil {
		return nil, err
	} else if supersededDisp != nil {
		defer supersededDisp.Release()
		if iUpdate.SupersededUpdateIDs, err = iStringCollectionToStringArray(supersededDisp); err != nil {
			return nil, err
		}
	}

	if iUpdate.SupportUrl, err = getPropertyString(updateDisp, "SupportUrl"); err != nil {
		return nil, err
	}

	if iUpdate.Title, err = getPropertyString(updateDisp, "Title"); err != nil {
		return nil, err
	}

	if uninstallBehaviorDisp, err := getPropertyDispatch(updateDisp, "UninstallationBehavior"); err != nil {
		return nil, err
	} else if uninstallBehaviorDisp != nil {
		defer uninstallBehaviorDisp.Release()
		if iUpdate.UninstallationBehavior, err = toIInstallationBehavior(uninstallBehaviorDisp); err != nil {
			return nil, err
		}
	}

	if iUpdate.UninstallationNotes, err = getPropertyString(updateDisp, "UninstallationNotes"); err != nil {
		return nil, err
	}

	if uninstallStepsDisp, err := getPropertyDispatch(updateDisp, "UninstallationSteps"); err != nil {
		return nil, err
	} else if uninstallStepsDisp != nil {
		defer uninstallStepsDisp.Release()
		if iUpdate.UninstallationSteps, err = iStringCollectionToStringArray(uninstallStepsDisp); err != nil {
			return nil, err
		}
	}

	return iUpdate, nil
}

//nolint:unused
func toIUpdateCollection(updates []*IUpdate) (*ole.IDispatch, error) {
	unknown, err := oleutil.CreateObject("Microsoft.Update.UpdateColl")
	if err != nil {
		return nil, err
	}
	defer unknown.Release()

	coll, err := unknown.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		return nil, err
	}
	for _, update := range updates {
		_, err := oleutil.CallMethod(coll, "Add", update.disp)
		if err != nil {
			return nil, err
		}
	}
	return coll, nil
}

// AcceptEula accepts the Microsoft Software License Terms that are associated with Windows Update. Administrators and power users can call this method.
// https://docs.microsoft.com/en-us/windows/win32/api/wuapi/nf-wuapi-iupdate-accepteula
func (iUpdate *IUpdate) AcceptEula() error {
	_, err := oleutil.CallMethod(iUpdate.disp, "AcceptEula")
	return err
}

// Release frees the underlying COM object.
func (iUpdate *IUpdate) Release() {
	if iUpdate.disp != nil {
		iUpdate.disp.Release()
		iUpdate.disp = nil
	}
}
