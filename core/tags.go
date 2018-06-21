package core

type Tag string

const (
	ReadenT   Tag = `\Seen`
	AnsweredT Tag = `\Answered`
)

func (c *Client) Tag(accountId, dir string, tag Tag, uids ...uint32) error {
	c.caches[accountId].lock.Lock()
	defer c.caches[accountId].lock.Unlock()

	err := c.imapConns[accountId].Tag(dir, string(tag), uids...)
	if err != nil {
		return err
	}

	for _, uid := range uids {
		msgPtr := c.caches[accountId].messagesByUid[dir][uid]
		if tag == ReadenT {
			msgPtr.Readen = true
		} else if tag == AnsweredT {
			msgPtr.Answered = true
		} else {
			// We don't want tag duplication.
			for _, t := range msgPtr.CustomTags {
				if t == string(tag) {
					// Tag is already present.
					return nil
				}
			}
			msgPtr.CustomTags = append(msgPtr.CustomTags, string(tag))
		}
	}
	return nil
}

func (c *Client) Untag(accountId, dir string, tag Tag, uids ...uint32) error {
	c.caches[accountId].lock.Lock()
	defer c.caches[accountId].lock.Unlock()

	err := c.imapConns[accountId].UnTag(dir, string(tag), uids...)
	if err != nil {
		return err
	}

	for _, uid := range uids {
		msgPtr := c.caches[accountId].messagesByUid[dir][uid]
		if tag == ReadenT {
			msgPtr.Readen = false
		} else if tag == AnsweredT {
			msgPtr.Answered = false
		} else {
			// Filter-out specified tag from cache, even if duplicated.
			newTags := []string{}
			for _, t := range msgPtr.CustomTags {
				if t != string(tag) {
					newTags = append(newTags, t)
				}
			}
			msgPtr.CustomTags = newTags
		}
	}
	return nil
}
