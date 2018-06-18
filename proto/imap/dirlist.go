package imap

import (
	imap "github.com/emersion/go-imap"
)

func (c *Client) DirList() (delimiter string, list []string, err error) {
	mailboxes := make(chan *imap.MailboxInfo, 32)
	done := make(chan error, 1)
	go func() {
		done <- c.cl.List("", "*", mailboxes)
	}()

	res := []string{}
	for m := range mailboxes {
		res = append(res, m.Name)
		delimiter = m.Delimiter
	}
	return delimiter, res, <-done
}

func (c *Client) DirStatus(dirName string) (total uint, unread uint, err error) {
	status, err := c.cl.Select(dirName, true)
	if err != nil {
		return 0, 0, nil
	}
	defer c.cl.Close()

	return uint(status.Messages), uint(status.Unseen), nil
}
