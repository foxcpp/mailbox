package imap

import (
	imap "github.com/emersion/go-imap"
)

func (c *Client) DirList() ([]string, error) {
	mailboxes := make(chan *imap.MailboxInfo, 32)
	done := make(chan error, 1)
	go func() {
		done <- c.cl.List("", "*", mailboxes)
	}()

	if c.seenMailboxes == nil {
		c.seenMailboxes = make(map[string]imap.MailboxInfo)
	}

	res := []string{}
	for m := range mailboxes {
		res = append(res, m.Name)
		c.seenMailboxes[m.Name] = *m
	}
	return res, <-done
}

func (c *Client) DirStatus(dirName string) (total uint, unread uint, err error) {
	status, err := c.cl.Select(dirName, true)
	if err != nil {
		return 0, 0, nil
	}

	return uint(status.Messages), uint(status.Unseen), nil
}
