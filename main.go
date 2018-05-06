package main

import (
	"fmt"
	"os"

	"github.com/Sirupsen/logrus"
	"github.com/docker/go-plugins-helpers/sdk"
)

const socketName = "/run/docker/plugins/logzio.sock"

var logLevels = map[string]logrus.Level{
	"debug": logrus.DebugLevel,
	"info":  logrus.InfoLevel,
	"warn":  logrus.WarnLevel,
	"error": logrus.ErrorLevel,
}

func main() {
	logrus.Info("Plugin socket is located at %s\n", socketName) // TODO - delete
	levelVal := os.Getenv("LOG_LEVEL")
	if levelVal == "" {
		levelVal = "info"
	}
	if level, exists := logLevels[levelVal]; exists {
		logrus.SetLevel(level)
	} else {
		fmt.Fprintln(os.Stderr, "invalid log level: ", levelVal)
		os.Exit(1)
	}
	h := sdk.NewHandler(`{"Implements": ["LoggingDriver"]}`)
	handlers(&h, newDriver())
	if err := h.ServeUnix(socketName, 0); err != nil {
		panic(err)
	}
}
