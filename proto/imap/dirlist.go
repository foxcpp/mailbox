package imap

import (
	imap "github.com/emersion/go-imap"
)

func (c *Client) DirList() (delimiter string, list []string, err error) {
	c.stopIdle()
	defer c.resumeIdle()
	c.IOLock.Lock()
	defer c.IOLock.Unlock()

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

type DirStatus = imap.MailboxStatus

func (c *Client) Status(dir string) (*DirStatus, error) {
	c.IOLock.Lock()
	defer c.IOLock.Unlock()

	mbox, err := c.cl.Select(dir, false)
	if err != nil {
		c.currentMailbox = ""
		return nil, err
	}
	return mbox, nil
}
