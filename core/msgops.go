package core

func (c *Client) MoveMsgs(accountId, fromDir, toDir string, uids ...uint32) error {
	err := c.imapConns[accountId].MoveTo(fromDir, toDir, uids...)
	if err == nil {
		for _, uid := range uids {
			c.caches[accountId].Dir(fromDir).DelMsg(uid)
		}
		c.reloadMaillist(accountId, toDir)
	}
	return err
}

func (c *Client) CopyMsgs(accountId, fromDir, toDir string, uids ...uint32) error {
	err := c.imapConns[accountId].CopyTo(fromDir, toDir, uids...)
	if err == nil {
		c.reloadMaillist(accountId, toDir)
	}
	return err
}

// DelMsg deletes message or moves
func (c *Client) DelMsg(accountId, dir string, skipTrash bool, uids ...uint32) error {
	if dir == "Trash" || skipTrash {
		err := c.imapConns[accountId].Delete(dir, uids...)
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
