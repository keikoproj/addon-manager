package controller

import (
	"time"

	"github.com/argoproj/argo-workflows/v3/util/env"
	"k8s.io/client-go/util/workqueue"
)

const (
	EnvVarDefaultRequeueTime = "DEFAULT_REQUEUE_TIME"
)

// default requeue time 10 seconds
func GetRequeueTime() time.Duration {
	return env.LookupEnvDurationOr(EnvVarDefaultRequeueTime, 10*time.Second)
}

type fixedItemIntervalRateLimiter struct{}

func (r *fixedItemIntervalRateLimiter) When(interface{}) time.Duration {
	return GetRequeueTime()
}

func (r *fixedItemIntervalRateLimiter) Forget(interface{}) {}

func (r *fixedItemIntervalRateLimiter) NumRequeues(interface{}) int {
	return 1
}

var _ workqueue.RateLimiter = &fixedItemIntervalRateLimiter{}
