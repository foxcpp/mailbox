package core

type Tag string

const (
	ReadenTag   Tag = `\Seen`
	AnsweredTag Tag = `\Answered`
)

func (c *Client) Tag(accountId, dir string, tag Tag, uids ...uint32) error {
	var err error
	for i := 0; i < *c.GlobalCfg.Connection.MaxTries; i++ {
		err = c.imapConns[accountId].Tag(c.rawDirName(accountId, dir), string(tag), uids...)
		if err == nil || !connectionError(err) {
			break
		}
		if err := c.connectToServer(accountId); err != nil {
			return err
		}
	}
	if err != nil {
		return err
	}

	for _, uid := range uids {
		c.caches[accountId].Dir(dir).AddTag(uid, string(tag))
	}
	return nil
}

func (c *Client) UnTag(accountId, dir string, tag Tag, uids ...uint32) error {
	var err error
	for i := 0; i < *c.GlobalCfg.Connection.MaxTries; i++ {
		err = c.imapConns[accountId].UnTag(c.rawDirName(accountId, dir), string(tag), uids...)
		if err == nil || !connectionError(err) {
			break
		}
		if err := c.connectToServer(accountId); err != nil {
			return err
		}
	}
	if err != nil {
		return err
	}

	for _, uid := range uids {
		c.caches[accountId].Dir(dir).RemTag(uid, string(tag))
	}
	return nil
}
