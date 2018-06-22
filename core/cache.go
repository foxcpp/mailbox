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
	for i, _ := range msgs {
		// This is done to ensure that there is exactly one copy
		// of message stored in memory.
		msg := &c.caches[accountId].messagesByDir[dir][i]
		c.caches[accountId].messagesByUid[dir][msg.UID] = msg
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
	Logger.Printf("Synchronizing cache for dir %v on account %v...\n", dir, accountId)

	uidvalidity, err := c.imapConns[accountId].UidValidity(dir)
	if err != nil {
		Logger.Printf("Failed to get UIDVALIDITY for %v on %v, asumming that cache is valid: %v\n", dir, accountId, err)
		return
	}
	if uidvalidity != c.caches[accountId].uidValidity[dir] {
		Logger.Println(uidvalidity, c.caches[accountId].uidValidity[dir])
		Logger.Printf("UIDVALIDITY value changed, discarding cache for directory %v on account %v\n", dir, accountId)
		c.caches[accountId].lock.Lock()
		delete(c.caches[accountId].messagesByDir, dir)
		delete(c.caches[accountId].messagesByUid, dir)
		c.caches[accountId].lock.Unlock()
		go c.prefetchData(accountId)
		return
	}

	// This will ask proto/imap to give us new messages.
	c.imapConns[accountId].KnownMailboxSizes[dir] = uint32(len(c.caches[accountId].messagesByDir[dir]))
	c.imapConns[accountId].ReplayUpdates(dir)
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
