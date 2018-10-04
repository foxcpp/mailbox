package core

import (
	"time"

	"github.com/foxcpp/mailbox/proto/common"
	"github.com/foxcpp/mailbox/proto/smtp"
)

// SaveDraft saves "draft" message to account's draft directory.
//
// draft argument must not be null. Invalid accountId leads to undefined behavior (probably panic).
//
// UID of saved message is returned.
func (c *Client) SaveDraft(accountId string, draft *common.Msg) (uint32, error) {
	draftDir := c.Accounts[accountId].Dirs.Drafts

	var uid uint32
	var err error
	for i := 0; i < *c.GlobalCfg.Connection.MaxTries; i++ {
		uid, err = c.imapConns[accountId].Create(c.rawDirName(accountId, draftDir), []string{`\Draft`}, time.Now(), draft)
		if err == nil || !connectionError(err) {
			break
		}
		if err := c.connectToServer(accountId); err != nil {
			return 0, err
		}
	}
	if err != nil {
		return 0, err
	}

	// Server is free to modify message somehow so we can't just
	// add message to cache. Fortunately, drafts directory rarely
	// grows big so it's ok to just reload it.
	//
	// TODO: Do servers actually modify messages? Is there anything
	// we can't predict (like, flags automatically set by server)?
	c.reloadMaillist(accountId, draftDir)

	return uid, nil
}

// UpdateDraft replaces old draft message with new with different contents.
//
// draft argument must not be null. Invalid accountId leads to undefined behavior (probably panic).
//
// Old message is removed and new one is created because IMAP doesn't allows to change existing
// messages. If error happens - older message is preserved.
func (c *Client) UpdateDraft(accountId string, oldUid uint32, new *common.Msg) (uint32, error) {
	draftDir := c.Accounts[accountId].Dirs.Drafts

	var uid uint32
	var err error
	for i := 0; i < *c.GlobalCfg.Connection.MaxTries; i++ {
		uid, err = c.imapConns[accountId].Replace(c.rawDirName(accountId, draftDir), oldUid, []string{`\Draft`}, time.Now(), new)
		if err == nil || !connectionError(err) {
			break
		}
		if err := c.connectToServer(accountId); err != nil {
			return 0, err
		}
	}
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
	c.logger.Printf("Connecting to SMTP server (%v:%v)...\n",
		c.serverCfgs[accountId].smtp.Host,
		c.serverCfgs[accountId].smtp.Port)
	client, err := smtp.Connect(c.serverCfgs[accountId].smtp)
	if err != nil {
		c.logger.Println("Connection failed:", err)
		return 0, err
	}
	c.logger.Println("Authenticating to SMTP server...")
	err = client.Auth(c.serverCfgs[accountId].smtp)
	if err != nil {
		c.logger.Println("Authentication failed:", err)
		return 0, err
	}

	err = client.Send(*msg)
	if err != nil {
		return 0, err
	}

	if *c.Accounts[accountId].CopyToSent {
		var uid uint32
		var err error
		for i := 0; i < *c.GlobalCfg.Connection.MaxTries; i++ {
			uid, err = c.imapConns[accountId].Create(c.rawDirName(accountId, c.Accounts[accountId].Dirs.Sent), []string{`\Seen`}, time.Now(), msg)
			if err == nil || !connectionError(err) {
				break
			}
			if err := c.connectToServer(accountId); err != nil {
				return 0, err
			}
		}
		if err != nil {
			c.logger.Printf("Failed to copy message to Sent (%v) directory: %v", c.Accounts[accountId].Dirs.Sent, err)
		}
		return uid, nil
	}
	return 0, nil
}
