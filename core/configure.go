package core

import "github.com/foxcpp/mailbox/storage"

func (c *Client) AddAccount(name string, conf storage.AccountCfg, updateConfig bool) *AccountError {
	c.Accounts[name] = conf

	c.prepareServerConfig(name)

	err := c.connectToServer(name)
	if err != nil {
		return err
	}

	c.initCaches(name)
	go c.prefetchData(name)

	if updateConfig {
		Logger.Println("Writting configuration for account", name+"...")
		return &AccountError{name, storage.SaveAccount(name, conf)}
	}
	return nil
}

func (c *Client) RemoveAccount(name string, updateConfig bool) error {
	// Sync. watchers shutdown.
	c.caches[name].updatesWatcherSig <- true
	<-c.caches[name].updatesWatcherSig
	c.caches[name].cacheCleanerSig <- true
	<-c.caches[name].cacheCleanerSig

	delete(c.caches, name)
	delete(c.Accounts, name)
	c.imapConns[name].Close()
	delete(c.imapConns, name)
	c.smtpConns[name].Close()
	delete(c.smtpConns, name)

	if updateConfig {
		return storage.DeleteAccount(name)
	}

	return nil
}
