/*
Copyright © 2023 Thibaud Demay <thibaud.demay@alyseo.com>
*/

package metrics

import "github.com/prometheus/client_golang/prometheus"

type gpioFanControlMetrics struct {
	gpioState     *prometheus.GaugeVec
	temperature   *prometheus.GaugeVec
	thresholdTemp *prometheus.GaugeVec
	criticalTemp  *prometheus.GaugeVec
	refreshTime   *prometheus.GaugeVec
	gpioPin       string
	sensorPath    string
}

func (m *gpioFanControlMetrics) SetGpioState(value float64) {
	m.gpioState.WithLabelValues(m.gpioPin, m.sensorPath).Set(value)
}

func (m *gpioFanControlMetrics) SetTemperature(value float64) {
	m.temperature.WithLabelValues(m.gpioPin, m.sensorPath).Set(value)
}

func (m *gpioFanControlMetrics) SetThresholdTemp(value float64) {
	m.thresholdTemp.WithLabelValues(m.gpioPin, m.sensorPath).Set(value)
}

func (m *gpioFanControlMetrics) SetCriticalTemp(value float64) {
	m.criticalTemp.WithLabelValues(m.gpioPin, m.sensorPath).Set(value)
}

func (m *gpioFanControlMetrics) SetRefreshTime(value float64) {
	m.refreshTime.WithLabelValues(m.gpioPin, m.sensorPath).Set(value)
}

func NewGpioFanControlMetrics(gpioPin string, sensorPath string) *gpioFanControlMetrics {
	m := &gpioFanControlMetrics{
		gpioState: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpio_fan_control_gpio_state",
			Help: "GPIO state for the fan.",
		}, []string{"gpio_pin", "sensor_path"}),
		temperature: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpio_fan_control_temperature",
			Help: "Current temperature.",
		}, []string{"gpio_pin", "sensor_path"}),
		thresholdTemp: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpio_fan_control_threshold_temp",
			Help: "Temperature to start the fan.",
		}, []string{"gpio_pin", "sensor_path"}),
		criticalTemp: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpio_fan_control_critical_temp",
			Help: "Temperature to shutdown system.",
		}, []string{"gpio_pin", "sensor_path"}),
		refreshTime: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gpio_fan_control_refresh_time",
			Help: "Time between each temperature check.",
		}, []string{"gpio_pin", "sensor_path"}),
		gpioPin:    gpioPin,
		sensorPath: sensorPath,
	}
	prometheus.MustRegister(m.gpioState)
	prometheus.MustRegister(m.temperature)
	prometheus.MustRegister(m.thresholdTemp)
	prometheus.MustRegister(m.criticalTemp)
	prometheus.MustRegister(m.refreshTime)
	return m
}
