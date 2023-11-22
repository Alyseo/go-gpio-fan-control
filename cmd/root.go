package cmd

import (
	"fmt"
	"go-gpio-fan-control/pkg/util/config"
	"go-gpio-fan-control/pkg/util/logging"
	"go-gpio-fan-control/pkg/util/metrics"
	"go-gpio-fan-control/pkg/util/version"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/apex/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/warthog618/gpiod"
)

var (
	verbose          bool
	gpio             string
	thresholdTemp    float64
	criticalTemp     float64
	refreshTime      time.Duration
	sensorPath       string
	criticalShutdown bool
	metricsEnabled   bool
	metricsPort      uint16
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "go-fan-control",
	Short: "Fan control using gpio",
	Long:  `Fan control for SBC, using GPIO control, and sysfs for read temperature from sensors.`,
	// Uncomment the following line if your bare application
	// has an action associated with it:
	Run: func(cmd *cobra.Command, args []string) {
		logger := logging.GetLogger()
		metricsEnabled := viper.GetBool("metrics")
		terminate := make(chan struct{})

		if metricsEnabled {
			go fanControl(cmd, args, logger, terminate)
			go func() {
				<-terminate
				logger.Infof("Stopping metrics server")
				os.Exit(0)
			}()

			metrics.StartMetricsServer()
		} else {
			fanControl(cmd, args, logger, terminate)
		}
	},
}

// Execute adds all child commands to the root command and sets PersistentFlags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	defaultRefreshTime, _ := time.ParseDuration("5s")

	cobra.OnInitialize(config.InitConfig)

	rootCmd.PersistentFlags().StringVarP(&config.ConfigFile, "config", "f", "", "Config file (default is /etc/gpio-fan-control/gpio-fan-control.conf.yml)")

	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Verbose mode.")
	viper.BindPFlag("verbose", rootCmd.PersistentFlags().Lookup("verbose"))

	rootCmd.PersistentFlags().StringVarP(&gpio, "gpio", "g", "", "GPIO pin number where the fan is connected.")
	rootCmd.MarkFlagRequired("gpio")
	viper.BindPFlag("gpio", rootCmd.PersistentFlags().Lookup("gpio"))

	rootCmd.PersistentFlags().Float64VarP(&thresholdTemp, "threshold-temp", "t", 45.0, "Temperature in celsius to start the fan.")
	rootCmd.MarkFlagRequired("threshold-temp")
	viper.BindPFlag("thresholdTemp", rootCmd.PersistentFlags().Lookup("threshold-temp"))

	rootCmd.PersistentFlags().Float64VarP(&criticalTemp, "critical-temp", "c", 77.0, "Temperature in celsius to reboot system.")
	rootCmd.MarkFlagRequired("critical-temp")
	viper.BindPFlag("criticalTemp", rootCmd.PersistentFlags().Lookup("critical-temp"))

	rootCmd.PersistentFlags().DurationVarP(&refreshTime, "refresh-time", "r", defaultRefreshTime, "Time in seconds between each temperature check.")
	viper.BindPFlag("refreshTime", rootCmd.PersistentFlags().Lookup("refresh-time"))
	viper.SetDefault("refreshTime", defaultRefreshTime)

	rootCmd.PersistentFlags().StringVarP(&sensorPath, "sensor-path", "s", "/sys/class/thermal/thermal_zone0/temp", "SysFS path to the temperature sensor.")
	viper.BindPFlag("sensorPath", rootCmd.PersistentFlags().Lookup("sensor-path"))
	viper.SetDefault("sensorPath", "/sys/class/thermal/thermal_zone0/temp")

	rootCmd.PersistentFlags().BoolVarP(&criticalShutdown, "critical-shutdown", "d", false, "Use shutdown instead of reboot when critical temperature is reached.")
	viper.BindPFlag("criticalShutdown", rootCmd.PersistentFlags().Lookup("critical-shutdown"))
	viper.SetDefault("criticalShutdown", false)

	rootCmd.PersistentFlags().BoolVarP(&metricsEnabled, "metrics", "m", false, "Enable metrics.")
	viper.BindPFlag("metrics", rootCmd.PersistentFlags().Lookup("metrics"))
	viper.SetDefault("metrics", false)

	rootCmd.PersistentFlags().Uint16VarP(&metricsPort, "metrics-port", "p", 6560, "Port to expose metrics.")
	viper.BindPFlag("port", rootCmd.PersistentFlags().Lookup("port"))
}

// Fan Control Stuff

// fanControl function is the main loop for fan control.
// It will read temperature from sensor, and start/stop fan depending on temperature threshold.
func fanControl(cmd *cobra.Command, args []string, logger log.Logger, terminate chan<- struct{}) {
	gpio := viper.GetString("gpio")
	thresholdTemp := viper.GetFloat64("thresholdTemp")
	criticalTemp := viper.GetFloat64("criticalTemp")
	refreshTime := viper.GetDuration("refreshTime")
	sensorPath := viper.GetString("sensorPath")
	criticalShutdown := viper.GetBool("criticalShutdown")

	// Get GPIO chip and line from GPIO pin number
	gpioChip, gpioLine := getGpioChipAndLine(gpio)

	// Initialize GPIO state
	fanGpioValue := 0

	// Prometheus metrics initialization
	logger.Debugf("Build metrics context and set const values (Threshold temperature, Critial temperature, Refresh time).")
	promMetrics := metrics.NewGpioFanControlMetrics(gpio, sensorPath, thresholdTemp, criticalTemp, refreshTime.Seconds())
	promMetrics.SetGpioState(float64(fanGpioValue))

	logger.Infof("Version: %s", version.BuildVersion())

	logger.Infof("Starting fan control with following parameters:")
	logger.Infof("  GPIO pin for fan: %s (%s, %d)", gpio, gpioChip, gpioLine)
	logger.Infof("  Sensor path: %s", sensorPath)
	logger.Infof("  Threshold temperature: %.2f", thresholdTemp)
	logger.Infof("  Critical temperature: %.2f", criticalTemp)
	logger.Infof("  Refresh time: %d", refreshTime)

	logger.Debugf("Opening GPIO pin: %s (%s, %d)", gpio, gpioChip, gpioLine)
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
func getGpioChipAndLine(gpio string) (string, int) {
	gpioPinNumber, _ := strconv.Atoi(gpio)
	gpioChipNumber := gpioPinNumber / 32
	gpioChip := fmt.Sprintf("gpiochip%d", gpioChipNumber)
	gpioLine := gpioPinNumber % 32
	return gpioChip, gpioLine
}
