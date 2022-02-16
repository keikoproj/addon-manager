package controllers

import (
	"github.com/keikoproj/addon-manager/pkg/addon"
)

type CtrlConfig struct {
	statusCache addon.AddOnWFStatusCache
}

func NewCtrlConfig() CtrlConfig {
	return CtrlConfig{
		statusCache: addon.NewAddOnStatusCache(),
	}
}
