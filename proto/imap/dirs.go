package imap

func (c *Client) CreateDir(name string) error {
	return c.cl.Create(name)
}

func (c *Client) RenameDir(from, to string) error {
	return c.cl.Rename(from, to)
}

func (c *Client) RemoveDir(name string) error {
	return c.cl.Delete(name)
}
