package addon

import (
	"strings"
	"sync"
)

type AddOnWFStatus interface {
	Update(wfnamespace, wfname, status string) error
	Read(wfnamespace, wfname, status string) error
	//Delete(wfnamespace, wfname, status string) error
}

type AddOnWFStatusCache struct {
	AddOnStatus map[string]map[string]map[string]string
	lock        *sync.Mutex
}

func NewAddOnStatusCache() AddOnWFStatusCache {
	return AddOnWFStatusCache{
		AddOnStatus: map[string]map[string]map[string]string{},
		lock:        &sync.Mutex{},
	}
}

// wfIdentifierName := fmt.Sprintf("%s-%s-%s-wf", prefix, lifecycleStep, a.CalculateChecksum())
func (ac *AddOnWFStatusCache) Update(wfnamespace, wfname, status string) {
	info := strings.Split(wfname, "-")
	addonName := info[0]
	lifecycle := info[1]

	ac.lock.Lock()
	defer ac.lock.Unlock()
	if _, ok := ac.AddOnStatus[wfnamespace]; !ok {
		ac.AddOnStatus[wfnamespace] = make(map[string]map[string]string)
	}
	if _, ok := ac.AddOnStatus[wfnamespace][addonName]; !ok {
		ac.AddOnStatus[wfnamespace][addonName] = make(map[string]string)
	}
	ac.AddOnStatus[wfnamespace][addonName][lifecycle] = status
}

func (ac *AddOnWFStatusCache) Read(namespace, addonname, lifecycle string) string {

	ac.lock.Lock()
	defer ac.lock.Unlock()
	if _, ok := ac.AddOnStatus[namespace]; !ok {
		return ""
	}
	if _, ok := ac.AddOnStatus[namespace][addonname]; !ok {
		return ""
	}
	if _, ok := ac.AddOnStatus[namespace][addonname][lifecycle]; !ok {
		return ""
	}
	return ac.AddOnStatus[namespace][addonname][lifecycle]
}
