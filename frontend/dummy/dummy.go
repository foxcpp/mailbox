// Quick and dirty "minimal frontend" for core testing.
// Set following environment variables before using:
// - MAILBOX_OFFLINE_ACCOUNT - Account for which all messages from INBOX will be downloaded.
// - MAILBOX_PASSWORD - Password used for all accounts.
package main

import (
	"fmt"
	"github.com/foxcpp/mailbox/core"
	"os"
	"os/signal"
	"syscall"
)

func passwordPrompt(prompt string) string {
	return string(os.Getenv("MAILBOX_PASSWORD"))
}

func main() {
	c, _ := core.Launch(core.FrontendHooks{
		PasswordPrompt: passwordPrompt,
	}, os.Stderr)

	fmt.Println("done with launch")
	c.DownloadOfflineDirs(os.Getenv("MAILBOX_OFFLINE_ACCOUNT"))

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGHUP, syscall.SIGTERM)
	select {
	case <-sig:
		c.Stop()
	}
}
