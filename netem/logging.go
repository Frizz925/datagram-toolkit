package netem

import (
	"os"

	"github.com/sirupsen/logrus"
)

var log = &logrus.Logger{
	Out:   os.Stderr,
	Level: logrus.WarnLevel,
	Formatter: &logrus.TextFormatter{
		FullTimestamp: true,
	},
}
