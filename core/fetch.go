package core

import (
	"fmt"
	"strings"

	"github.com/foxcpp/mailbox/proto/imap"
)

func (c *Client) GetDirs(accountId string) (StrSet, error) {
	c.caches[accountId].lock.Lock()
	defer c.caches[accountId].lock.Unlock()

	set := c.caches[accountId].dirs
	if set != nil {
		// Cache hit!
		return set, nil
	}

	Logger.Printf("Cache miss in GetDirs for %v\n", accountId)
	// Cache miss, go and ask server.
	// TODO: Interrupt watcher if needed.
	separator, list, err := c.imapConns[accountId].DirList()
	if err != nil {
		Logger.Printf("GetDirs failed (%v): %v\n", accountId, err)
		return nil, fmt.Errorf("dirs %v: %v", accountId, err)
	}

	c.imapDirSep = separator
	resSet := make(StrSet)
	for _, name := range list {
		resSet.Add(c.normalizeDirName(name))
	}

	c.caches[accountId].dirs = resSet
	return resSet, nil
}

// Normalized dir name - directory name with all server-defined path
// delimiters replaced with our server-independent separator (currently "|").

func (c *Client) normalizeDirName(raw string) string {
	return strings.Replace(raw, c.imapDirSep, "|", -1)
}

func (c *Client) rawDirName(normalized string) string {
	return strings.Replace(normalized, "|", c.imapDirSep, -1)
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
	_, count, err := c.imapConns[accountId].DirStatus(c.rawDirName(dirName))
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
	list, err := c.imapConns[accountId].FetchMaillist(c.rawDirName(dirName))
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
