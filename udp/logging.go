package udp

import (
	"io/ioutil"
	"log"
	"os"
)

var (
	discardLogger = log.New(ioutil.Discard, "", 0)
	stderrLogger  = log.New(os.Stderr, "", 0)
)
