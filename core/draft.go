package core

import (
	"time"

	"github.com/foxcpp/mailbox/proto/common"
	"github.com/foxcpp/mailbox/proto/smtp"
)

func (c *Client) SaveDraft(accountId string, draft *common.Msg) (uint32, error) {
	draftDir := c.Accounts[accountId].Dirs.Drafts

	uid, err := c.imapConns[accountId].Create(draftDir, []string{`\Draft`}, time.Now(), draft)
	if err != nil {
		return 0, err
	}

	c.reloadMaillist(accountId, draftDir)

	return uid, nil
}

func (c *Client) UpdateDraft(accountId string, oldUid uint32, new *common.Msg) (uint32, error) {
	draftDir := c.Accounts[accountId].Dirs.Drafts

	uid, err := c.imapConns[accountId].Replace(draftDir, oldUid, []string{`\Draft`}, time.Now(), new)
	if err != nil {
		return 0, err
	}

	c.reloadMaillist(accountId, draftDir)

	return uid, nil
}

// SendMessage of course... Sends message!
//
// Recipient and other important information is parsed from message headers.
// Function will return UID of message copy placed in Sent directory, if any
// and zero if user disabled this.
func (c *Client) SendMessage(accountId string, msg *common.Msg) (uint32, error) {
	Logger.Printf("Connecting to SMTP server (%v:%v)...\n",
		c.serverCfgs[accountId].smtp.Host,
		c.serverCfgs[accountId].smtp.Port)
	client, err := smtp.Connect(c.serverCfgs[accountId].smtp)
	if err != nil {
		Logger.Println("Connection failed:", err)
		return 0, err
	}
	Logger.Println("Authenticating to SMTP server...")
	err = client.Auth(c.serverCfgs[accountId].smtp)
	if err != nil {
		Logger.Println("Authentication failed:", err)
		return 0, err
	}

	err = client.Send(*msg)
	if err != nil {
		return 0, err
	}

	if *c.Accounts[accountId].CopyToSent {
		uid, err := c.imapConns[accountId].Create(c.Accounts[accountId].Dirs.Sent, []string{`\Seen`}, time.Now(), msg)
		if err != nil {
			Logger.Printf("Failed to copy message to Sent (%v) directory: %v", c.Accounts[accountId].Dirs.Sent, err)
		}
		return uid, nil
	}
	return 0, nil
}
