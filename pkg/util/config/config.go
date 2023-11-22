package config

import (
	"go-gpio-fan-control/pkg/util/logging"

	"github.com/spf13/viper"
)

var (
	ConfigFile        string
	defaultConfigPath = "/etc/gpio-fan-control/"
	defaultConfigFile = "gpio-fan-control.conf.yml"
)

func InitConfig() {
	logger := logging.GetLogger()
	if ConfigFile != "" {
		logger.Debugf("Opening config file: %s", ConfigFile)
		viper.SetConfigFile(ConfigFile)
	} else {
		viper.AddConfigPath(defaultConfigPath)
		viper.SetConfigName(defaultConfigFile)
		viper.SetConfigType("yaml")
	}

	viper.ReadInConfig()
}
