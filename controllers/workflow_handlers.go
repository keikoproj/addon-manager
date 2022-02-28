package controllers

import (
	"context"
	"fmt"

	"github.com/keikoproj/addon-manager/pkg/common"
	addonwfutility "github.com/keikoproj/addon-manager/pkg/workflows"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func (c *Controller) handleWorkFlowUpdate(ctx context.Context, obj interface{}) error {
	c.logger.Info("workflow update")

	wfobj, err := common.WorkFlowFromUnstructured(obj.(*unstructured.Unstructured))
	if err != nil {
		msg := fmt.Sprintf("converting to workflow object err %#v", err)
		c.logger.Info(msg)
		return fmt.Errorf(msg)
	}

	// check the associated addons and update its status
	msg := fmt.Sprintf("workflow %s/%s update status.phase %s", wfobj.GetNamespace(), wfobj.GetName(), wfobj.Status.Phase)
	c.logger.Info(msg)

	if len(string(wfobj.Status.Phase)) == 0 {
		c.logger.Info("skip workflow ", wfobj.GetNamespace(), "/", wfobj.GetName(), " empty status update.")
		return nil
	}

	// find the Addon from the namespace and update its status accordingly
	addonName, lifecycle, err := addonwfutility.ExtractAddOnNameAndLifecycleStep(wfobj.GetName())
	if err != nil {
		msg := fmt.Sprintf("could not extract addon/lifecycle from %s/%s workflow.",
			wfobj.GetNamespace(),
			wfobj.GetName())
		c.logger.Info(msg)
		return fmt.Errorf(msg)
	}

	return c.updateAddonStatusLifecycle(ctx, wfobj.GetNamespace(), addonName, lifecycle, wfobj.Status.Phase)
}

func (c *Controller) handleWorkFlowAdd(ctx context.Context, obj interface{}) error {
	wfobj, err := common.WorkFlowFromUnstructured(obj.(*unstructured.Unstructured))
	if err != nil {
		msg := fmt.Sprintf("\n### converting to workflow object %#v", err)
		c.logger.Info(msg)
		return fmt.Errorf(msg)
	}

	// check the associated addons and update its status
	msg := fmt.Sprintf("workflow %s/%s update status.phase %s", wfobj.GetNamespace(), wfobj.GetName(), wfobj.Status.Phase)
	c.logger.Info(msg)
	if len(string(wfobj.Status.Phase)) == 0 {
		msg := fmt.Sprintf("skip %s/%s workflow empty status.", wfobj.GetNamespace(),
			wfobj.GetName())
		c.logger.Info(msg)
		return nil
	}
	addonName, lifecycle, err := addonwfutility.ExtractAddOnNameAndLifecycleStep(wfobj.GetName())
	if err != nil {
		msg := fmt.Sprintf("could not extract addon/lifecycle from %s/%s workflow.",
			wfobj.GetNamespace(),
			wfobj.GetName())
		c.logger.Info(msg)
		return fmt.Errorf(msg)
	}

	return c.updateAddonStatusLifecycle(ctx, wfobj.GetNamespace(), addonName, lifecycle, wfobj.Status.Phase)
}
