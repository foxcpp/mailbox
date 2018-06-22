package imap

import (
	eimap "github.com/emersion/go-imap"
)

func (c *Client) FetchMaillist(dir string) ([]MessageInfo, error) {
	c.stopIdle()
	defer c.resumeIdle()
	c.IOLock.Lock()
	defer c.IOLock.Unlock()

	mbox, err := c.cl.Select(dir, true)
	if err != nil {
		return nil, err
	}
	defer c.cl.Close()

	if mbox.Messages == 0 {
		return []MessageInfo{}, nil
	}

	seqset := new(eimap.SeqSet)
	seqset.AddRange(1, mbox.Messages)

	out := make(chan *eimap.Message, 16)
	done := make(chan error)
	go func() {
		done <- c.cl.Fetch(seqset, []eimap.FetchItem{eimap.FetchEnvelope, eimap.FetchUid}, out)
	}()

	res := []MessageInfo{}
	for msg := range out {
		res = append(res, MessageToInfo(msg))
	}
	return res, <-done
}

func min(a, b uint32) uint32 {
	if a < b {
		return a
	}
	return b
}

func (c *Client) FetchPartialMaillist(dir string, count, offset uint32) ([]MessageInfo, error) {
	c.stopIdle()
	defer c.resumeIdle()
	c.IOLock.Lock()
	defer c.IOLock.Unlock()

	mbox, err := c.cl.Select(dir, true)
	if err != nil {
		return nil, err
	}

	if mbox.Messages == 0 {
		return []MessageInfo{}, nil
	}

	seqset := eimap.SeqSet{}
	seqset.AddRange(1+offset, min(1+offset+count, mbox.Messages))

	out := make(chan *eimap.Message, 16)
	done := make(chan error)
	go func() {
		done <- c.cl.Fetch(&seqset, []eimap.FetchItem{eimap.FetchEnvelope, eimap.FetchUid}, out)
	}()

	res := []MessageInfo{}
	for msg := range out {
		res = append(res, MessageToInfo(msg))
	}
	return res, <-done
}
