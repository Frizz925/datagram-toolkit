package muxer

import (
	"os"

	"github.com/sirupsen/logrus"
)

var log = &logrus.Logger{
	Out:   os.Stdout,
	Level: logrus.DebugLevel,
	Formatter: &logrus.TextFormatter{
		FullTimestamp: true,
	},
}
