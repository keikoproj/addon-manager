package controllers

import (
	"context"

	"k8s.io/client-go/dynamic/dynamicinformer"
)

type WfInformers struct {
	nsInformers dynamicinformer.DynamicSharedInformerFactory

	stopCh <-chan struct{}
}

func NewWfInformers(nsInfo dynamicinformer.DynamicSharedInformerFactory, stopCh <-chan struct{}) *WfInformers {
	return &WfInformers{
		nsInformers: nsInfo,
		stopCh:      stopCh,
	}
}

func (wfinfo *WfInformers) Start(ctx context.Context) error {
	s := wfinfo.stopCh
	wfinfo.nsInformers.Start(s)
	wfinfo.nsInformers.WaitForCacheSync(s)
	<-wfinfo.stopCh
	return nil
}
