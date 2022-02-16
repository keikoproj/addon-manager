package addon

import (
	"fmt"
	"sync"

	addonwfutility "github.com/keikoproj/addon-manager/pkg/workflows"
)

type AddOnWFStatus interface {
	Update(wfnamespace, wfname, status string) error
	Read(wfnamespace, wfname, status string) error
	Delete(wfnamespace, wfname string) error
	Add(wfnamespace, wfname, status string) error
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
	addonName, lifecycle, err := addonwfutility.ExtractAddOnNameAndLifecycleStep(wfname)
	if err != nil {
		return
	}
	ac.lock.Lock()
	defer ac.lock.Unlock()
	if _, ok := ac.AddOnStatus[wfnamespace]; !ok {
		ac.AddOnStatus[wfnamespace] = make(map[string]map[string]string)
	}
	if _, ok := ac.AddOnStatus[wfnamespace][addonName]; !ok {
		ac.AddOnStatus[wfnamespace][addonName] = make(map[string]string)
	}

	ac.AddOnStatus[wfnamespace][addonName][lifecycle] = status
	fmt.Printf("\n Persist %s:%s:%s:%s \n", wfnamespace, addonName, lifecycle, status)
}

func (ac *AddOnWFStatusCache) Add(wfnamespace, wfname, status string) {
	addonName, lifecycle, err := addonwfutility.ExtractAddOnNameAndLifecycleStep(wfname)
	if err != nil {
		return
	}
	ac.lock.Lock()
	defer ac.lock.Unlock()
	if _, ok := ac.AddOnStatus[wfnamespace]; !ok {
		ac.AddOnStatus[wfnamespace] = make(map[string]map[string]string)
	}
	if _, ok := ac.AddOnStatus[wfnamespace][addonName]; !ok {
		ac.AddOnStatus[wfnamespace][addonName] = make(map[string]string)
	}

	if len(status) == 0 {
		ac.AddOnStatus[wfnamespace][addonName][lifecycle] = "Pending"
	} else {
		ac.AddOnStatus[wfnamespace][addonName][lifecycle] = status
	}
	fmt.Printf("\n Persist %s:%s:%s:%s \n", wfnamespace, addonName, lifecycle, ac.AddOnStatus[wfnamespace][addonName][lifecycle])
}

func (ac *AddOnWFStatusCache) Read(namespace, addonname, lifecycle string) string {
	fmt.Printf("\n Reading  %s:%s:%s \n", namespace, addonname, lifecycle)

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
	fmt.Printf("\n Read Return %s:%s:%s:%s \n", namespace, addonname, lifecycle, ac.AddOnStatus[namespace][addonname][lifecycle])
	return ac.AddOnStatus[namespace][addonname][lifecycle]

}

func (ac *AddOnWFStatusCache) Delete(wfnamespace, wfname string) {
	fmt.Printf("\n Deleting workflow  %s:%s \n", wfnamespace, wfname)
	ac.lock.Lock()
	defer ac.lock.Unlock()

	if _, ok := ac.AddOnStatus[wfnamespace]; !ok {
		return
	}

	if _, ok := ac.AddOnStatus[wfnamespace][wfname]; !ok {
		return
	}

	ac.AddOnStatus[wfnamespace][wfname] = make(map[string]string)
	fmt.Printf("\n Deleted workflow  %s:%s \n", wfnamespace, wfname)
}
