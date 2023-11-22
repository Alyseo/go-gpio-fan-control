package logging

import (
	"os"

	"github.com/apex/log"
	"github.com/apex/log/handlers/cli"
	"github.com/spf13/viper"
)

var logger *log.Logger

func initLogging() *log.Logger {
	wantedLevel, _ := log.ParseLevel("info")
	verbose := viper.GetBool("verbose")
	if verbose {
		wantedLevel, _ = log.ParseLevel("debug")
	}

	logger := log.Logger{
		Handler: cli.New(os.Stdout),
		Level:   wantedLevel,
	}
	return &logger
}

func GetLogger() log.Logger {
	if logger == nil {
		logger = initLogging()
	}
	return *logger
}
