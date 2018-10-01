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
	})

	fmt.Println("done with launch")
	c.DownloadOfflineDirs("cock")

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGHUP, syscall.SIGTERM)
	select {
	case <-sig:
		c.Stop()
	}
}
