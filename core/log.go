package core

import (
	"log"
	"os"
)

var Logger *log.Logger

func init() {
	Logger = log.New(os.Stderr, "[mailbox/core] ", log.LstdFlags)
}
