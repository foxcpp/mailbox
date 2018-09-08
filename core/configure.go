package core

import (
	"path/filepath"

	"github.com/foxcpp/mailbox/storage"
)

func (c *Client) AddAccount(name string, conf storage.AccountCfg, updateConfig bool) *AccountError {
	c.Accounts[name] = conf

	c.prepareServerConfig(name)

	err := c.connectToServer(name)
	if err != nil {
		return err
	}

	var dberr error
	c.caches[name], dberr = storage.OpenCacheDB(filepath.Join(storage.GetDirectory(), "accounts", name+".db"))
	if dberr != nil {
		return &AccountError{name, err}
	}
	if lst, err := c.caches[name].DirList(); err == nil || len(lst) == 0 {
		c.prefetchData(name)
	}

	if updateConfig {
		Logger.Println("Writting configuration for account", name+"...")
		return &AccountError{name, storage.SaveAccount(name, conf)}
	}
	return nil
}

func (c *Client) RemoveAccount(name string, updateConfig bool) error {
	c.caches[name].Close()
	delete(c.caches, name)
	delete(c.Accounts, name)
	c.imapConns[name].Close()
	delete(c.imapConns, name)

	if updateConfig {
		return storage.DeleteAccount(name)
	}

	return nil
}
