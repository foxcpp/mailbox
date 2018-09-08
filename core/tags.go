package core

type Tag string

const (
	ReadenT   Tag = `\Seen`
	AnsweredT Tag = `\Answered`
)

func (c *Client) Tag(accountId, dir string, tag Tag, uids ...uint32) error {
	err := c.imapConns[accountId].Tag(dir, string(tag), uids...)
	if err != nil {
		return err
	}

	for _, uid := range uids {
		c.caches[accountId].Dir(dir).AddTag(uid, string(tag))
	}
	return nil
}

func (c *Client) Untag(accountId, dir string, tag Tag, uids ...uint32) error {
	err := c.imapConns[accountId].UnTag(dir, string(tag), uids...)
	if err != nil {
		return err
	}

	for _, uid := range uids {
		c.caches[accountId].Dir(dir).RemTag(uid, string(tag))
	}
	return nil
}
