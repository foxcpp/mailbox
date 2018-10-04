package core

import (
	"path/filepath"

	"github.com/foxcpp/mailbox/storage"
)

// AddAccount creates new account from configuration.
//
// LoadAccount should be called manually after AddAccount.
func (c *Client) AddAccount(name string, conf storage.AccountCfg) {
	c.debugLog.Println("Writing configuration for account", name+"...")
	storage.SaveAccount(name, conf)
}

// LoadAccount initializes all structures requires for operations with this account and
// opens connection to IMAP server.
//
// UnloadAccount is called automatically to clean-up partially initialized account
// in case of error.
func (c *Client) LoadAccount(name string, conf storage.AccountCfg) *AccountError {
	c.Accounts[name] = conf
	c.prepareServerConfig(name)

	var dberr error
	c.caches[name], dberr = storage.OpenCacheDB(filepath.Join(storage.GetDirectory(), "accounts", name+".db"))
	if dberr != nil {
		c.UnloadAccount(name)
		return &AccountError{name, dberr}
	}

	err := c.connectToServer(name)
	if err != nil {
		c.UnloadAccount(name)
		return err
	}
	c.prefetchData(name)
	// TODO: c.setSpecialUseDirs()

	return nil
}

// UnloadAccount frees all resources associated with account.
//
// After this operation account no longer can be used in any Client methods
// before corresponding LoadAccount or Client restart.
func (c *Client) UnloadAccount(name string) {
	conn := c.imapConns[name]
	delete(c.imapConns, name)
	if conn != nil {
		conn.Close()
	}

	cache := c.caches[name]
	delete(c.caches, name)
	if cache != nil {
		cache.Close()
	}

	delete(c.Accounts, name)
	delete(c.serverCfgs, name)

	c.imapDirSep.Delete(name)
	delete(c.prefetchDirs, name)
}

// DeleteAccount deletes account from configuration.
func (c *Client) DeleteAccount(name string) error {
	c.UnloadAccount(name)
	return storage.DeleteAccount(name)
}
