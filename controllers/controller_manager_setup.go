package controllers

import (
	"context"
	"fmt"

	addonapiv1 "github.com/keikoproj/addon-manager/api/addon"
	"github.com/keikoproj/addon-manager/pkg/addon"
	"github.com/keikoproj/addon-manager/pkg/common"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

func New(mgr manager.Manager) error {
	versionCache := addon.NewAddonVersionCacheClient()
	dynClient := dynamic.NewForConfigOrDie(mgr.GetConfig())
	nsInformers := dynamicinformer.NewFilteredDynamicSharedInformerFactory(dynClient, addonapiv1.AddonResyncPeriod, addonapiv1.ManagedNameSpace, nil)
	wfInf := nsInformers.ForResource(common.WorkflowGVR()).Informer()
	if err := mgr.Add(manager.RunnableFunc(func(ctx context.Context) error {
		nsInformers.Start(ctx.Done())
		nsInformers.WaitForCacheSync(ctx.Done())
		return nil
	})); err != nil {
		return fmt.Errorf("failed to run informer sync: %w", err)
	}

	if _, err := NewAddonController(mgr, dynClient, wfInf, versionCache); err != nil {
		return fmt.Errorf("failed to create addon controller: %w", err)
	}

	if _, err := NewWFController(mgr, dynClient, wfInf, versionCache); err != nil {
		return fmt.Errorf("failed to create addon wf controller: %w", err)
	}

	return nil
}
