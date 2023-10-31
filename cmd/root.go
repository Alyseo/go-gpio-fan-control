/*
Copyright © 2023 Thibaud Demay <thibaud.demay@alyseo.com>
*/
package cmd

import (
	"fmt"
	"go-gpio-fan-control/metrics"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/apex/log"
	"github.com/apex/log/handlers/cli"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"
	"github.com/warthog618/gpiod"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "go-fan-control",
	Short: "Fan control using gpio",
	Long:  `Fan control for SBC, using GPIO control, and sysfs for read temperature from sensors.`,
	// Uncomment the following line if your bare application
	// has an action associated with it:
	Run: func(cmd *cobra.Command, args []string) {
		portFromArgs, _ := cmd.Flags().GetUint16("port")
		port := fmt.Sprintf(":%d", portFromArgs)
		terminate := make(chan struct{})

		logger := initLogging(cmd)
		go fanControl(cmd, args, logger, terminate)
		go func() {
			<-terminate
			logger.Infof("Stopping metrics server")
			os.Exit(0)
		}()

		logger.Infof("Starting metrics server on 0.0.0.0%s/metrics", port)
		http.Handle("/metrics", promhttp.Handler())
		http.ListenAndServe(port, nil)
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	defaultRefreshTime, _ := time.ParseDuration("5s")
	rootCmd.Flags().StringP("gpio", "g", "70", "GPIO pin number where the fan is connected.")
	rootCmd.Flags().Float64P("threshold-temp", "t", 45.0, "Temperature in celsius to start the fan.")
	rootCmd.Flags().Float64P("critical-temp", "c", 77.0, "Temperature in celsius to reboot system.")
	rootCmd.Flags().DurationP("refresh-time", "r", defaultRefreshTime, "Time in seconds between each temperature check.")
	rootCmd.Flags().StringP("sensor-path", "s", "/sys/class/thermal/thermal_zone0/temp", "SysFS path to the temperature sensor.")
	rootCmd.Flags().Uint16P("port", "p", 6560, "Port to expose metrics.")
	rootCmd.Flags().BoolP("critical-shutdown", "d", false, "Use shutdown instead of reboot when critical temperature is reached.")
	rootCmd.Flags().BoolP("verbose", "v", false, "Verbose mode.")
}

// Initiliaze logging and manage verbosity
func initLogging(cmd *cobra.Command) log.Logger {
	verbose, _ := cmd.Flags().GetBool("verbose")
	wantedLevel, _ := log.ParseLevel("info")
	if verbose {
		wantedLevel, _ = log.ParseLevel("debug")
	}

	logger := log.Logger{
		Handler: cli.New(os.Stdout),
		Level:   wantedLevel,
	}
	return logger
}

// Fan Control Stuff

// fanControl function is the main loop for fan control.
// It will read temperature from sensor, and start/stop fan depending on temperature threshold.
func fanControl(cmd *cobra.Command, args []string, logger log.Logger, terminate chan<- struct{}) {
	// Get parameters from command line
	sensorPath, _ := cmd.Flags().GetString("sensor-path")
	gpioPin, _ := cmd.Flags().GetString("gpio")
	thresholdTemp, _ := cmd.Flags().GetFloat64("threshold-temp")
	criticalTemp, _ := cmd.Flags().GetFloat64("critical-temp")
	refreshTime, _ := cmd.Flags().GetDuration("refresh-time")
	criticalShutdown, _ := cmd.Flags().GetBool("critical-shutdown")

	// Get GPIO chip and line from GPIO pin number
	gpioChip, gpioLine := getGpioChipAndLine(gpioPin)

	// Initialize GPIO state
	fanGpioValue := 0

	// Prometheus metrics initialization
	logger.Debugf("Build metrics context and set const values (Threshold temperature, Critial temperature, Refresh time).")
	promMetrics := metrics.NewGpioFanControlMetrics(gpioPin, sensorPath)
	promMetrics.SetThresholdTemp(thresholdTemp)
	promMetrics.SetCriticalTemp(criticalTemp)
	promMetrics.SetRefreshTime(refreshTime.Seconds())
	promMetrics.SetGpioState(float64(fanGpioValue))

	logger.Infof("Starting fan control with following parameters:")
	logger.Infof("  GPIO pin for fan: %s (%s, %d)", gpioPin, gpioChip, gpioLine)
	logger.Infof("  Sensor path: %s", sensorPath)
	logger.Infof("  Threshold temperature: %.2f", thresholdTemp)
	logger.Infof("  Critical temperature: %.2f", criticalTemp)
	logger.Infof("  Refresh time: %d", refreshTime)

	logger.Debugf("Opening GPIO pin: %s (%s, %d)", gpioPin, gpioChip, gpioLine)
	// Request GPIO line and set initial value
	fanGpio, err := gpiod.RequestLine(gpioChip, gpioLine, gpiod.AsOutput(fanGpioValue))
	if err != nil {
		logger.Errorf("Cannot open GPIO pin: %s", err)
		panic(err)
	}

	// Open sensor file in sysfs
	logger.Debugf("Opening sensor file: %s", sensorPath)
	sensorFile, err := os.Open(sensorPath)
	if err != nil {
		logger.Errorf("Cannot open sensor file: %s", err)
		panic(err)
	}

	// Close GPIO line and sensor file when exiting
	defer func() {
		logger.Infof("Stopping fan control")
		sensorFile.Close()
		fanGpio.SetValue(0)
		fanGpio.Close()
		close(terminate)
	}()

	// Handle SIGINT and SIGTERM
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	logger.Debugf("Starting fan control loop")
	var temp float64
	for {
		select {
		case <-time.After(refreshTime):
			temp = getTempFromFile(sensorFile)
			promMetrics.SetTemperature(temp)

			logger.Debugf("Current temperature: %.2f", temp)

			if temp >= criticalTemp {
				logger.Errorf("Critical temperature reached: %.2f", temp)
				if !criticalShutdown {
					syscall.Reboot(syscall.LINUX_REBOOT_CMD_RESTART)
				} else {
					syscall.Reboot(syscall.LINUX_REBOOT_CMD_POWER_OFF)
				}
			} else if temp >= thresholdTemp && fanGpioValue == 0 {
				logger.Infof("Starting fan, temperature: %.2f", temp)
				fanGpioValue = 1
				fanGpio.SetValue(fanGpioValue)
				promMetrics.SetGpioState(float64(fanGpioValue))
			} else if temp < thresholdTemp && fanGpioValue == 1 {
				logger.Infof("Stopping fan, temperature: %.2f", temp)
				fanGpioValue = 0
				fanGpio.SetValue(fanGpioValue)
				promMetrics.SetGpioState(float64(fanGpioValue))
			}
		case <-quit:
			return
		}
	}
}

// Read temperature from sensor on file
// Temperature is in milli-celsius, so we divide by 1000 to get celsius
func getTempFromFile(sensorFile *os.File) float64 {
	var temp float64
	buf := make([]byte, 5)
	sensorFile.ReadAt(buf, 0)
	temp, _ = strconv.ParseFloat(string(buf), 64)
	temp = temp / 1000.0
	return temp
}

// Get GPIO chip and line from GPIO pin number
// GPIO pin number is the number of the pin on the board
func getGpioChipAndLine(gpioPin string) (string, int) {
	gpioPinNumber, _ := strconv.Atoi(gpioPin)
	gpioChipNumber := gpioPinNumber / 32
	gpioChip := fmt.Sprintf("gpiochip%d", gpioChipNumber)
	gpioLine := gpioPinNumber % 32
	return gpioChip, gpioLine
}
