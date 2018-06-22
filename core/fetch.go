package core

import (
	"fmt"
	"strings"

	"github.com/foxcpp/mailbox/proto/common"
	"github.com/foxcpp/mailbox/proto/imap"
)

// GetDirs returns list of *all* directories for specified account.
//
// Nested directories are returned with name in form:
//  Parent|Child
// If there is Archive directory with 2018/2017 subdirs and 2018 contains Work
// subdir then GetDirs will return following:
// - Archive
// - Archive|2017
// - Archive|2018
// - Archive|2018|Work
//
// Function arguments are NOT checked for validity, invalid account ID will
// lead to undefined behavior (usually panic).
func (c *Client) GetDirs(accountId string) (StrSet, error) {
	c.caches[accountId].lock.Lock()
	defer c.caches[accountId].lock.Unlock()

	set := c.caches[accountId].dirs
	if set != nil {
		// Cache hit!
		return set, nil
	}

	Logger.Printf("Downloading directories list for %v...\n", accountId)
	// Cache miss, go and ask server.
	separator, list, err := c.imapConns[accountId].DirList()
	if err != nil {
		Logger.Printf("Directories list download (%v) failed: %v\n", accountId, err)
		return nil, fmt.Errorf("dirs %v: %v", accountId, err)
	}

	c.imapDirSep = separator
	resSet := make(StrSet)
	for _, name := range list {
		resSet.Add(c.normalizeDirName(name))
	}

	c.caches[accountId].dirs = resSet
	c.caches[accountId].dirty = true
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

// GetUnreadCount returns amount of unread messages in specified directory.
//
// Returned value is cached, it's fine to call it repeatly.
// Function arguments are NOT checked for validity, invalid account ID or
// directory name will lead to undefined behavior (usually panic).
func (c *Client) GetUnreadCount(accountId, dirName string) (uint, error) {
	c.caches[accountId].lock.Lock()
	defer c.caches[accountId].lock.Unlock()

	count, prs := c.caches[accountId].unreadCounts[dirName]
	if prs {
		// Cache hit!
		return count, nil
	}

	// Cache miss, go and ask server.
	_, count, err := c.imapConns[accountId].DirStatus(c.rawDirName(dirName))
	if err != nil {
		Logger.Printf("Directories status download (%v, %v) failed: %v\n", accountId, dirName, err)
		return 0, fmt.Errorf("unreadcount %v, %v: %v", accountId, dirName, err)
	}

	c.caches[accountId].unreadCounts[dirName] = count

	return count, nil
}

// GetMsgsList returns *full* list of messages in specified directory.
// Each entry includes base headers (From, To, Cc, Bcc, Date, Subject), IMAP flags and
// message UID.
//
// Returned value is cached, it's fine to call it repeatly.
// Function arguments are NOT checked for validity, invalid account ID or
// directory name will lead to undefined behavior (usually panic).
func (c *Client) GetMsgsList(accountId, dirName string) ([]imap.MessageInfo, error) {
	return c.getMsgsList(accountId, dirName, false)
}

func (c *Client) getMsgsList(accountId, dirName string, forceDownload bool) ([]imap.MessageInfo, error) {
	c.caches[accountId].lock.Lock()
	defer c.caches[accountId].lock.Unlock()

	list, prs := c.caches[accountId].messagesByDir[dirName]
	if prs && !forceDownload {
		// Cache hit!
		return list, nil
	}

	Logger.Printf("Downloading message list for %v, %v...\n", accountId, dirName)
	// Cache miss, go and ask server.
	list, err := c.imapConns[accountId].FetchMaillist(c.rawDirName(dirName))
	if err != nil {
		Logger.Printf("Message list download (%v, %v) failed: %v\n", accountId, dirName, err)
		return nil, fmt.Errorf("msgslist %v, %v: %v", accountId, dirName, err)
	}

	oldMsgsByUid := c.caches[accountId].messagesByUid[dirName]

	c.caches[accountId].messagesByDir[dirName] = list
	c.caches[accountId].messagesByUid[dirName] = make(map[uint32]*imap.MessageInfo)
	for i, msg := range list {
		c.caches[accountId].messagesByUid[dirName][msg.UID] = &list[i]
		if oldMsg, prs := oldMsgsByUid[msg.UID]; prs {
			c.caches[accountId].messagesByUid[dirName][msg.UID].Msg.Parts = oldMsg.Msg.Parts
		}
	}
	c.caches[accountId].dirty = true

	return list, nil
}

// GetMsgText returns all message headers (not only limited set like
// GetMsgsList does) + text parts (with MIME type text/*). Information about
// non-text parts is present but Body slice is nil.
//
// Returned value is cached if allowOutdated is true, it's fine to
// call it repeatly. Function arguments are NOT checked for validity, invalid
// account ID or directory name will lead to undefined behavior (usually
// panic).
func (c *Client) GetMsgText(accountId, dirName string, uid uint32, allowOutdated bool) (*common.Msg, error) {
	c.caches[accountId].lock.Lock()
	defer c.caches[accountId].lock.Unlock()

	if allowOutdated {
		if msg, prs := c.caches[accountId].messagesByUid[dirName][uid]; prs && len(msg.Msg.Parts) != 0 {
			return &msg.Msg, nil
		}
	}

	Logger.Printf("Downloading message text for (%v, %v, %v)...\n", accountId, dirName, uid)
	msg, err := c.imapConns[accountId].FetchPartialMail(dirName, uid, imap.TextOnly)
	if err != nil {
		Logger.Printf("Message text download (%v, %v, %v) failed: %v\n", accountId, dirName, uid, err)
		return nil, fmt.Errorf("msgtext %v, %v, %v: %v", accountId, dirName, uid, err)
	}

	// Update information in cache.
	// TODO: Update other information.
	c.caches[accountId].messagesByUid[dirName][uid].Msg.Parts = msg.Msg.Parts
	c.caches[accountId].dirty = true

	return &c.caches[accountId].messagesByUid[dirName][uid].Msg, nil
}

// GetMsgPart downloads message part specified by part index (literally index
// of element in Parts slice got form GetMsgText).
//
// Returned value is NOT cached, you should be careful to not call this
// function more than needed. Function arguments are NOT checked for validity,
// invalid account ID or directory name will lead to undefined behavior
// (usually anic).
func (c *Client) GetMsgPart(accountId, dirName string, uid uint32, partIndex int) (*common.Part, error) {
	return c.imapConns[accountId].DownloadPart(dirName, uid, partIndex)
}

func (c *Client) ResolveUid(accountId, dir string, seqnum uint32) (uint32, error) {
	//return c.caches[accountId].messagesByDir[dir][seqnum].UID, nil
	return c.imapConns[accountId].ResolveUid(dir, seqnum)
}

func (c *Client) DownloadOfflineDirs(accountId string) {
	Logger.Println("Downloading messages for offline use...")
	for _, dir := range c.Accounts[accountId].Dirs.DownloadForOffline {
		list, err := c.GetMsgsList(accountId, dir)
		if err != nil {
			return
		}
		for _, msg := range list {
			c.GetMsgText(accountId, dir, msg.UID, true)
		}
	}
}
