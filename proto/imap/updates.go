package imap

import (
	eimap "github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
)

func (c *Client) updatesWatch() {
	lastMbox := ""
	for {
		select {
		case update := <-c.updates:
			switch update.(type) {
			case *client.MailboxUpdate:
				mboxUpd := update.(*client.MailboxUpdate)
				if _, prs := c.KnownMailboxSizes[mboxUpd.Mailbox.Name]; !prs {
					// We didn't seen this mailbox before, just record size.
					c.KnownMailboxSizes[mboxUpd.Mailbox.Name] = mboxUpd.Mailbox.Messages
					continue
				}
				knownSize := c.KnownMailboxSizes[mboxUpd.Mailbox.Name]
				if knownSize < mboxUpd.Mailbox.Messages {
					// There are new messages we didn't seen before!
					if c.Callbacks != nil {
						for i := knownSize + 1; i <= mboxUpd.Mailbox.Messages; i++ {
							c.Callbacks.NewMessage(mboxUpd.Mailbox.Name, i)
						}
					}
				}
				c.KnownMailboxSizes[mboxUpd.Mailbox.Name] = mboxUpd.Mailbox.Messages
			case *client.ExpungeUpdate:
				// XXX: This still can explode when current mailbox != mailbox when update
				// was received.
				if c.cl.Mailbox() != nil {
					lastMbox = c.cl.Mailbox().Name
				}
				c.KnownMailboxSizes[lastMbox] -= 1
				if c.Callbacks != nil {
					c.Callbacks.MessageRemoved(lastMbox, update.(*client.ExpungeUpdate).SeqNum)
				}
			case *client.MessageUpdate:
				// XXX: This still can explode when current mailbox != mailbox when update
				// was received.
				if c.cl.Mailbox() != nil {
					lastMbox = c.cl.Mailbox().Name
				}
				if c.Callbacks != nil {
					c.Callbacks.MessageUpdate(lastMbox, update.(*client.MessageUpdate).Message)
				}
			}
		case <-c.updatesDispatcherStop:
			c.updatesDispatcherStop <- true
			return
		}
	}
}

func (c *Client) ResolveUid(dir string, seqnum uint32) (uint32, error) {
	c.stopIdle()
	defer c.resumeIdle()
	c.IOLock.Lock()
	defer c.IOLock.Unlock()

	_, err := c.cl.Select(dir, true)
	if err != nil {
		return 0, err
	}
	defer c.cl.Close()

	seqset := eimap.SeqSet{}
	seqset.AddNum(seqnum)

	out := make(chan *eimap.Message, 1)
	err = c.cl.Fetch(&seqset, []eimap.FetchItem{eimap.FetchUid}, out)
	if err != nil {
		return 0, err
	}
	return (<-out).Uid, nil
}

func (c *Client) ReplayUpdates(dir string) error {
	// Just select and deselect mailbox, our update dispatcher will take care
	// of updates.
	_, err := c.cl.Select(dir, true)
	if err != nil {
		return err
	}
	c.cl.Close()
	return nil
}

func (c *Client) UidValidity(dir string) (uint32, error) {
	mbox, err := c.cl.Select(dir, true)
	if err != nil {
		return 0, err
	}
	defer c.cl.Close()
	return mbox.UidValidity, nil
}
