package core

type Tag string

const (
	ReadenT   Tag = `\Seen`
	AnsweredT Tag = `\Answered`
)

func (c *Client) Tag(accountId, dir string, tag Tag, uids ...uint32) error {
	var err error
	for i := 0; i < 5; i++ {
		err = c.imapConns[accountId].Tag(dir, string(tag), uids...)
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

func (c *Client) Untag(accountId, dir string, tag Tag, uids ...uint32) error {
	var err error
	for i := 0; i < 5; i++ {
		err = c.imapConns[accountId].UnTag(dir, string(tag), uids...)
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
