package controllers

import (
	"context"

	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/informers"
)

type WfInformers struct {
	generatedInformers informers.SharedInformerFactory
	nsInformers        dynamicinformer.DynamicSharedInformerFactory

	stopCh <-chan struct{}
}

func NewwfInformers(gInfo informers.SharedInformerFactory, nsInfo dynamicinformer.DynamicSharedInformerFactory, stopCh <-chan struct{}) *WfInformers {
	return &WfInformers{
		generatedInformers: gInfo,
		nsInformers:        nsInfo,
		stopCh:             stopCh,
	}
}

func (wfinfo *WfInformers) Start(ctx context.Context) error {
	s := wfinfo.stopCh
	wfinfo.generatedInformers.Start(s)
	wfinfo.generatedInformers.WaitForCacheSync(s)
	wfinfo.nsInformers.Start(s)
	wfinfo.nsInformers.WaitForCacheSync(s)
	<-wfinfo.stopCh
	return nil
}
