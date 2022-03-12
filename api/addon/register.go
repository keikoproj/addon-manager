package addon

import "time"

// Addon constants
const (
	Group          string = "addonmgr.keikoproj.io"
	Version        string = "v1alpha1"
	APIVersion     string = Group + "/" + Version
	AddonKind      string = "Addon"
	AddonSingular  string = "addon"
	AddonPlural    string = "addons"
	AddonShortName string = "addon"
	AddonFullName  string = AddonPlural + "." + Group

	ManagedNameSpace string = "addon-manager-system"

	AddonResyncPeriod = 20 * time.Minute

	FinalizerName = "delete.addonmgr.keikoproj.io"

	ResourceDefaultManageByLabel = "app.kubernetes.io/managed-by"
	ResourceDefaultManageByValue = "addonmgr.keikoproj.io"
	ResourceDefaultOwnLabel      = "app.kubernetes.io/name"
	ResourceDefaultPartLabel     = "app.kubernetes.io/part-of"

	TTL = time.Duration(1) * time.Hour // 1 hour

	// complete status - addon either succeed, failed, or error
	AddonCompleteLabel   = "addons.addonmgr.keikoproj.io/completed"
	AddonCompleteTrueKey = "true"
)
