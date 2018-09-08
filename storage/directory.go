package storage

import (
	"os"
	"path/filepath"
	"runtime"
)

// GetDirectory picks first path from following list if all envvars are present:
// - $MAILBOX_HOME
// - $USERPROFILE\AppData\Roaming\mailbox (Windows-only)
// - $XDG_CONFIG_HOME/mailbox
// - $HOME/.config/mailbox
func GetDirectory() string {
	res := os.Getenv("MAILBOX_HOME")
	if res == "" {
		if runtime.GOOS == "windows" {
			return filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Roaming", "mailbox")
		} else {
			cfgHome := os.Getenv("XDG_CONFIG_HOME")
			if cfgHome == "" {
				cfgHome = filepath.Join(os.Getenv("HOME"), ".config")
			}
			return filepath.Join(cfgHome, "mailbox")
		}
	}

	return res
}
