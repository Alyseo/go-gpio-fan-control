package metrics

import (
	"fmt"
	"go-gpio-fan-control/pkg/util/logging"
	"go-gpio-fan-control/pkg/util/version"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/viper"
)

type gpioFanControlMetrics struct {
	buildInfo     prometheus.GaugeFunc
	thresholdTemp prometheus.GaugeFunc
	criticalTemp  prometheus.GaugeFunc
	checkInterval prometheus.GaugeFunc
	gpioState     *prometheus.GaugeVec
	temperature   *prometheus.GaugeVec
	gpioPin       string
	sensorPath    string
}

func (m *gpioFanControlMetrics) SetGpioState(value float64) {
	m.gpioState.WithLabelValues(m.gpioPin, m.sensorPath).Set(value)
}

func (m *gpioFanControlMetrics) SetTemperature(value float64) {
	m.temperature.WithLabelValues(m.gpioPin, m.sensorPath).Set(value)
}

func NewGpioFanControlMetrics(gpioPin string, sensorPath string, thresholdTemp float64, criticalTemp float64, checkInterval float64) *gpioFanControlMetrics {
	commonLabels := prometheus.Labels{
		"gpio_pin":    gpioPin,
		"sensor_path": sensorPath,
	}
	m := &gpioFanControlMetrics{
		buildInfo: prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "gpio_fan_control_build_info",
			Help: "A metric with a constant '1' value labeled by version, commitHash, branch, buildTimestamp, builtBy.",
			ConstLabels: prometheus.Labels{
				"version":        version.Version,
				"commitHash":     version.CommitHash,
				"branch":         version.Branch,
				"buildTimestamp": version.BuildTimestamp,
				"builtBy":        version.BuiltBy,
			},
		}, func() float64 { return 1 }),
		thresholdTemp: prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name:        "gpio_fan_control_threshold_temp",
			Help:        "Temperature to start the fan.",
			ConstLabels: commonLabels,
		}, func() float64 { return thresholdTemp }),
		criticalTemp: prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name:        "gpio_fan_control_critical_temp",
			Help:        "Temperature to shutdown system.",
			ConstLabels: commonLabels,
		}, func() float64 { return criticalTemp }),
		checkInterval: prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name:        "gpio_fan_control_refresh_time",
			Help:        "Time between each temperature check.",
			ConstLabels: commonLabels,
		}, func() float64 { return checkInterval }),
		gpioState: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpio_fan_control_gpio_state",
			Help: "GPIO state for the fan.",
		}, []string{"gpio_pin", "sensor_path"}),
		temperature: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpio_fan_control_temperature",
			Help: "Current temperature.",
		}, []string{"gpio_pin", "sensor_path"}),
		gpioPin:    gpioPin,
		sensorPath: sensorPath,
	}
	prometheus.MustRegister(m.buildInfo)
	prometheus.MustRegister(m.thresholdTemp)
	prometheus.MustRegister(m.criticalTemp)
	prometheus.MustRegister(m.checkInterval)
	prometheus.MustRegister(m.gpioState)
	prometheus.MustRegister(m.temperature)
	return m
}

func StartMetricsServer() {
	logger := logging.GetLogger()
	port := viper.GetUint16("metricsPort")
	addr := fmt.Sprintf(":%d", port)
	logger.Infof("Starting metrics server on 0.0.0.0:%d/metrics", port)
	http.Handle("/metrics", promhttp.Handler())
	http.ListenAndServe(addr, nil)
}
