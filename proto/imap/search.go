package imap

import eimap "github.com/emersion/go-imap"

func (c *Client) Search(dir string, criteria eimap.SearchCriteria) ([]uint32, error) {
	c.stopIdle()
	defer c.resumeIdle()
	c.IOLock.Lock()
	defer c.IOLock.Lock()

	if _, err := c.cl.Select(dir, true); err != nil {
		return nil, err
	}

	return c.cl.UidSearch(&criteria)
}
