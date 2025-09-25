package updates

import (
	"math"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	statusLagGaugeOnce sync.Once
	statusLagGauge     *prometheus.GaugeVec
)

func recordStatusLag(consumer, lane string, lag time.Duration) {
	gauge := getStatusLagGauge()
	if consumer == "" {
		consumer = "unknown"
	}
	if lane == "" {
		lane = "unknown"
	}
	seconds := lag.Seconds()
	if seconds < 0 || math.IsNaN(seconds) {
		seconds = 0
	}
	gauge.WithLabelValues(consumer, lane).Set(seconds)
}

func getStatusLagGauge() *prometheus.GaugeVec {
	statusLagGaugeOnce.Do(func() {
		statusLagGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "ploy_updates_status_consumer_lag_seconds",
			Help: "Lag between self-update status event timestamp and CLI consumption time",
		}, []string{"consumer", "lane"})
	})
	return statusLagGauge
}
