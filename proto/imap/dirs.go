package imap

func (c *Client) CreateDir(name string) error {
	c.stopIdle()
	defer c.resumeIdle()
	c.IOLock.Lock()
	defer c.IOLock.Unlock()

	return c.cl.Create(name)
}

func (c *Client) RenameDir(from, to string) error {
	c.stopIdle()
	defer c.resumeIdle()
	c.IOLock.Lock()
	defer c.IOLock.Unlock()

	return c.cl.Rename(from, to)
}

func (c *Client) RemoveDir(name string) error {
	c.stopIdle()
	defer c.resumeIdle()
	c.IOLock.Lock()
	defer c.IOLock.Unlock()

	return c.cl.Delete(name)
}
