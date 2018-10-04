package imap

// CreateDir creates directory.
//
// No check is done to ensure that directory can be  actually created before
// attempting to do so.
//
// See https://tools.ietf.org/html/rfc3501#section-6.3.3 for details
// of underlying command semantics.
func (c *Client) CreateDir(name string) error {
	c.stopIdle()
	defer c.resumeIdle()
	c.IOLock.Lock()
	defer c.IOLock.Unlock()

	return c.cl.Create(name)
}

// RenameDir renames directory.
//
// See https://tools.ietf.org/html/rfc3501#section-6.3.5 for details
// of underlying command semantics.
func (c *Client) RenameDir(from, to string) error {
	c.stopIdle()
	defer c.resumeIdle()
	c.IOLock.Lock()
	defer c.IOLock.Unlock()

	err := c.cl.Rename(from, to)
	if err == nil {
		c.KnownMailboxSizes[to] = c.KnownMailboxSizes[from]
		delete(c.KnownMailboxSizes, from)
	}
	return err
}

// RemoveDir deletes directory from server.
//
// See https://tools.ietf.org/html/rfc3501#section-6.3.4 for details
// of underlying command semantics.
func (c *Client) RemoveDir(name string) error {
	c.stopIdle()
	defer c.resumeIdle()
	c.IOLock.Lock()
	defer c.IOLock.Unlock()

	err := c.cl.Delete(name)
	if err == nil {
		delete(c.KnownMailboxSizes, name)
	}
	return err
}
