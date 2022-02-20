package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

var AddonConditionMetric = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Namespace: addonNamespace,
		Name:      "addon_condition",
		Help:      "addon condition. ",
	},
	[]string{"type", "status"},
)
