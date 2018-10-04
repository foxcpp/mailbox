package core

// MoveMsgs moves messages from one directory to another.
//
// Operation can be expensive due to forced message list reload so it's recommended to call it
// in separate goroutine.
//
// Invalid UIDs are ignored. Invalid account ID leads to undefined behavior (probably panic).
// Invalid fromDir or toDir leads to error.
func (c *Client) MoveMsgs(accountId, fromDir, toDir string, uids ...uint32) error {
	err := c.imapConns[accountId].MoveTo(c.rawDirName(accountId, fromDir), c.rawDirName(accountId, toDir), uids...)
	if err == nil {
		for _, uid := range uids {
			c.caches[accountId].Dir(fromDir).DelMsg(uid)
		}
		c.reloadMaillist(accountId, toDir)
	}
	return err
}

// CopyMsgs copies messages from one directory to another.
//
// Operation can be expensive due to forced message list reload so it's recommended to call it
// in separate goroutine.
//
// Invalid UIDs are ignored. Invalid account ID leads to undefined behavior (probably panic).
// Invalid fromDir or toDir leads to error.
func (c *Client) CopyMsgs(accountId, fromDir, toDir string, uids ...uint32) error {
	err := c.imapConns[accountId].CopyTo(c.rawDirName(accountId, fromDir), toDir, uids...)
	if err == nil {
		c.reloadMaillist(accountId, toDir)
	}
	return err
}

// DelMsg deletes message or moves them to trash.
//
// Operation can be expensive due to forced message list reload so it's recommended to call it
// in separate goroutine.
//
// Invalid UIDs are ignored. Invalid account ID leads to undefined behavior (probably panic).
// Invalid fromDir or toDir leads to error.
// skipTrash=true disables moveement to Trash and just removes messages in any case.
func (c *Client) DelMsg(accountId, dir string, skipTrash bool, uids ...uint32) error {
	if dir == "Trash" || skipTrash {
		err := c.imapConns[accountId].Delete(c.rawDirName(accountId, dir), uids...)
		if err == nil {
			for _, uid := range uids {
				c.caches[accountId].Dir(dir).DelMsg(uid)
			}
		}
		return err
	} else {
		return c.MoveMsgs(accountId, dir, c.Accounts[accountId].Dirs.Trash, uids...)
	}
}
