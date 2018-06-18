package core

import (
	"fmt"

	"github.com/foxcpp/mailbox/proto/imap"
)

func (c *Client) GetDirs(accountId string) ([]string, error) {
	c.caches[accountId].lock.Lock()
	defer c.caches[accountId].lock.Unlock()

	list := c.caches[accountId].dirs
	if list != nil {
		// Cache hit!
		return list, nil
	}

	Logger.Printf("Cache miss in GetDirs for %v\n", accountId)
	// Cache miss, go and ask server.
	// TODO: Interrupt watcher if needed.
	list, err := c.imapConns[accountId].DirList()
	if err != nil {
		Logger.Printf("GetDirs failed (%v): %v\n", accountId, err)
		return nil, fmt.Errorf("dirs %v: %v", accountId, err)
	}

	c.caches[accountId].dirs = list
	return list, nil
}

func (c *Client) GetUnreadCount(accountId, dirName string) (uint, error) {
	c.caches[accountId].lock.Lock()
	defer c.caches[accountId].lock.Unlock()

	count, prs := c.caches[accountId].unreadCounts[dirName]
	if prs {
		// Cache hit!
		return count, nil
	}

	// Cache miss, go and ask server.
	// TODO: Interrupt watcher if needed.
	_, count, err := c.imapConns[accountId].DirStatus(dirName)
	if err != nil {
		Logger.Printf("GetUnreadCount failed (%v, %v): %v\n", accountId, dirName, err)
		return 0, fmt.Errorf("unreadcount %v, %v: %v", accountId, dirName, err)
	}

	c.caches[accountId].unreadCounts[dirName] = count

	return count, nil
}

func (c *Client) GetMsgsList(accountId, dirName string) ([]imap.MessageInfo, error) {
	c.caches[accountId].lock.Lock()
	defer c.caches[accountId].lock.Unlock()

	list, prs := c.caches[accountId].messagesByDir[dirName]
	if prs {
		// Cache hit!
		return list, nil
	}

	Logger.Printf("Cache miss in GetMsgsList for %v, %v\n", accountId, dirName)
	// Cache miss, go and ask server.
	// TODO: Interrupt watcher if needed.
	list, err := c.imapConns[accountId].FetchMaillist(dirName)
	if err != nil {
		Logger.Printf("GetMsgsList failed (%v, %v): %v\n", accountId, dirName, err)
		return nil, fmt.Errorf("msgslist %v, %v: %v", accountId, dirName, err)
	}

	c.caches[accountId].messagesByDir[dirName] = list
	for _, msg := range list {
		c.caches[accountId].messagesByUid[msg.UID] = &msg
	}

	return list, nil
}
