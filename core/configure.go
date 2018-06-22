package core

import (
	"github.com/foxcpp/mailbox/storage"
)

func (c *Client) AddAccount(name string, conf storage.AccountCfg, updateConfig bool) *AccountError {
	c.Accounts[name] = conf

	c.prepareServerConfig(name)

	c.initCaches(name)
	c.loadFullCache(name)

	err := c.connectToServer(name)
	if err != nil {
		return err
	}

	if len(c.caches[name].dirs.List()) == 0 {
		go c.prefetchData(name)
	} else {
		go c.resyncFullCache(name)
	}

	go c.cacheFlusher(name)

	if updateConfig {
		Logger.Println("Writting configuration for account", name+"...")
		return &AccountError{name, storage.SaveAccount(name, conf)}
	}
	return nil
}

func (c *Client) RemoveAccount(name string, updateConfig bool) error {
	c.caches[name].cacheFlusherStopSig <- true
	<-c.caches[name].cacheFlusherStopSig

	delete(c.caches, name)
	delete(c.Accounts, name)
	c.imapConns[name].Close()
	delete(c.imapConns, name)

	if updateConfig {
		return storage.DeleteAccount(name)
	}

	return nil
}
