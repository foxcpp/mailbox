package core

import (
	"time"

	"github.com/foxcpp/mailbox/proto/imap"
	"github.com/foxcpp/mailbox/storage"
)

func (c *Client) loadFullCache(accountId string) {
	for _, dir := range storage.CachedDirs(accountId) {
		c.loadCache(accountId, dir)
		if c.caches[accountId].dirs == nil {
			c.caches[accountId].dirs = make(StrSet)
		}
		c.caches[accountId].dirs.Add(dir)
	}
}

func (c *Client) loadCache(accountId, dir string) {
	Logger.Printf("Loading cache for dir %v on account %v...\n", dir, accountId)
	uidvalidity, msgs, err := storage.ReadCache(accountId, dir)
	if err != nil {
		Logger.Printf("Failed to read cache for dir %v on account %v: %v\n", dir, accountId, err)
		return
	}

	c.caches[accountId].uidValidity[dir] = uidvalidity
	c.caches[accountId].messagesByDir[dir] = msgs
	c.caches[accountId].messagesByUid[dir] = make(map[uint32]*imap.MessageInfo)
	for i, msg := range msgs {
		c.caches[accountId].messagesByUid[dir][msg.UID] = &c.caches[accountId].messagesByDir[dir][i]
	}
	c.caches[accountId].dirty = false
	Logger.Println(len(msgs), "messages in dir", dir)
}

func (c *Client) resyncFullCache(accountId string) {
	for _, dir := range c.caches[accountId].dirs.List() {
		c.resyncCache(accountId, dir)
	}
}

func (c *Client) resyncCache(accountId, dir string) {
	curUidVal, err := c.imapConns[accountId].UidValidity(dir)
	if err != nil {
		// This likely means that this directory doesn't exists anymore, remove it from cache.
		c.caches[accountId].dirs.Remove(dir)
		delete(c.caches[accountId].uidValidity, dir)
		delete(c.caches[accountId].messagesByDir, dir)
		delete(c.caches[accountId].messagesByDir, dir)
		delete(c.caches[accountId].unreadCounts, dir)
		storage.RemoveSavedCache(accountId, dir)
		return
	}
	if c.caches[accountId].uidValidity[dir] != curUidVal {
		Logger.Printf("UIDVALIDITY value changed, discarding cache for directory %v on account %v\n", dir, accountId)
		c.caches[accountId].uidValidity[dir] = curUidVal
		c.caches[accountId].messagesByDir[dir] = []imap.MessageInfo{}
		c.caches[accountId].messagesByUid[dir] = make(map[uint32]*imap.MessageInfo)
	}

	c.prefetchDirData(accountId, dir)
}

func (c *Client) saveFullCache(accountId string) error {
	for _, dir := range c.caches[accountId].dirs.List() {
		err := c.saveCache(accountId, dir)
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) saveCache(accountId, dir string) error {
	// Make sure we will not write inconsistent cache.
	c.caches[accountId].lock.Lock()
	defer c.caches[accountId].lock.Unlock()

	Logger.Printf("Saving cache for dir %v on account %v...\n", dir, accountId)
	err := storage.WriteCache(accountId, dir, c.caches[accountId].uidValidity[dir], c.caches[accountId].messagesByDir[dir])
	if err != nil {
		Logger.Printf("Failed to save cache for dir %v on account %v: %v\n", dir, accountId, err)
	}
	return err
}

func (c *Client) cacheFlusher(accountId string) {
	ticker := time.NewTicker(1 * time.Minute)
	for {
		select {
		case <-ticker.C:
			if c.caches[accountId].dirty {
				err := c.saveFullCache(accountId)
				if err == nil {
					c.caches[accountId].dirty = false
				}
			}
		case <-c.caches[accountId].cacheFlusherStopSig:
			if c.caches[accountId].dirty {
				err := c.saveFullCache(accountId)
				if err == nil {
					c.caches[accountId].dirty = false
				}
			}
			c.caches[accountId].cacheFlusherStopSig <- true
			ticker.Stop()
			return
		}
	}
}
