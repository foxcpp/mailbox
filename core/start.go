package core

import (
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"

	eimap "github.com/emersion/go-imap"
	"github.com/foxcpp/mailbox/proto/common"
	"github.com/foxcpp/mailbox/proto/imap"
	"github.com/foxcpp/mailbox/storage"
)

// FrontendHooks provides a way for core to call into GUI for various needs
// such as fatal error reporting or password requests.
//
// Functions may be called from multiple gorountines at the same time so it's
// generally a good idea to ensure thread-safety.
//
// Also, when initializing, use named initializers. New fields WILL BE added
// to this structure between minor releases.
type FrontendHooks struct {
	// Input is prompt text, output should be password.
	PasswordPrompt func(string) string

	// Called when due to some kind of de-sync all caches for specific account were invalidated.
	// Frontend should re-request all data from core and update presentation in UI.
	Reset func(string)

	// Same as Reset but only one directory is invalidated.
	ResetDir func(string, string)

	// Called when new message received.
	NewMessage func(string, string, *imap.MessageInfo)
}

type Client struct {
	SkippedAccounts []AccountError
	Hooks           FrontendHooks

	masterKey []byte
	Accounts  map[string]storage.AccountCfg
	GlobalCfg storage.GlobalCfg

	caches map[string]*storage.CacheDB

	serverCfgs map[string]struct {
		imap, smtp common.ServConfig
	}

	imapConns map[string]*imap.Client

	imapDirSep string
}

type AccountError struct {
	AccountId string
	Err       error
}

func (e AccountError) Error() string {
	return fmt.Sprintf("connect %v: %v", e.AccountId, e.Err)
}

// Launch reads client configuration and connects to servers.
func Launch(hooks FrontendHooks) (*Client, error) {
	res := new(Client)
	res.Hooks = hooks

	Logger.Println("Loading configuration...")
	globalCfg, err := storage.LoadGlobal()
	if err != nil {
		return nil, err
	}
	res.GlobalCfg = *globalCfg
	accounts, err := storage.LoadAllAccounts()
	if err != nil {
		return nil, err
	}

	mpass := ""
	if *res.GlobalCfg.Encryption.UseMasterPass {
		mpass = hooks.PasswordPrompt("Enter master password: ")
		if mpass == "" {
			return nil, errors.New("launch: password prompt rejected")
		}
	}
	err = res.prepareMasterKey("")
	if err != nil {
		return nil, errors.New("launch: failed to prepare master key")
	}

	res.serverCfgs = make(map[string]struct {
		imap, smtp common.ServConfig
	})

	res.Accounts = make(map[string]storage.AccountCfg)
	res.caches = make(map[string]*storage.CacheDB)
	res.imapConns = make(map[string]*imap.Client)

	for name, info := range accounts {
		Logger.Println("Setting up account", name+"...")
		err := res.AddAccount(name, info, false /* write config */)
		if err != nil {
			res.SkippedAccounts = append(res.SkippedAccounts, *err)
		}
	}

	// Save new config, in case something changed something.
	storage.SaveGlobal(&res.GlobalCfg)

	return res, nil
}

func (c *Client) Stop() {
	for name, _ := range c.Accounts {
		c.RemoveAccount(name, false)
	}
}

func (c *Client) prepareServerConfig(accountId string) {
	connTypeConv := func(s string) common.ConnType {
		if s == "tls" {
			return common.TLS
		}
		if s == "starttls" {
			return common.STARTTLS
		}
		// Config reader checks validity, so this should not really happen
		return common.STARTTLS
	}

	info := c.Accounts[accountId]

	pass := ""
	if len(info.Credentials.Pass) != 0 {
		encPass, err := hex.DecodeString(info.Credentials.Pass)
		if err != nil {
			pass = ""
		}
		passBytes, err := c.DecryptUsingMaster(encPass)
		if err != nil {
			pass = ""
		} else {
			pass = string(passBytes)
		}
	}
	if pass == "" && c.Hooks.PasswordPrompt != nil {
		pass = c.Hooks.PasswordPrompt("Enter password for " + info.SenderEmail + ":")
	}

	c.serverCfgs[accountId] = struct{ imap, smtp common.ServConfig }{
		imap: common.ServConfig{
			Host:     info.Server.Imap.Host,
			Port:     info.Server.Imap.Port,
			ConnType: connTypeConv(info.Server.Imap.Encryption),
			User:     info.Credentials.User,
			Pass:     pass,
		},
		smtp: common.ServConfig{
			Host:     info.Server.Smtp.Host,
			Port:     info.Server.Smtp.Port,
			ConnType: connTypeConv(info.Server.Smtp.Encryption),
			User:     info.Credentials.User,
			Pass:     pass,
		},
	}
}

func (c *Client) connectToServer(accountId string) *AccountError {
	var err error

	if c.imapConns[accountId] != nil {
		if err := c.imapConns[accountId].Reconnect(c.serverCfgs[accountId].imap); err != nil {
			return &AccountError{accountId, err}
		}
		if err := c.imapConns[accountId].Auth(c.serverCfgs[accountId].imap); err != nil {
			return &AccountError{accountId, err}
		}
		return nil
	}

	Logger.Printf("Connecting to IMAP server (%v:%v)...\n",
		c.serverCfgs[accountId].imap.Host,
		c.serverCfgs[accountId].imap.Port)
	c.imapConns[accountId], err = imap.Connect(c.serverCfgs[accountId].imap)
	if err != nil {
		Logger.Println("Connection failed:", err)
		return &AccountError{accountId, err}
	}
	Logger.Println("Authenticating to IMAP server...")
	err = c.imapConns[accountId].Auth(c.serverCfgs[accountId].imap)
	if err != nil {
		Logger.Println("Authentication failed:", err)
		return &AccountError{accountId, err}
	}

	c.imapConns[accountId].Callbacks = c.makeUpdateCallbacks(accountId)
	c.imapConns[accountId].Logger = *log.New(os.Stderr, "[mailbox/proto/imap:"+accountId+"] ", log.LstdFlags)

	return nil
}

func (c *Client) makeUpdateCallbacks(accountId string) *imap.UpdateCallbacks {
	return &imap.UpdateCallbacks{
		NewMessage: func(dir string, seqnum uint32) {
			Logger.Printf("New message for account %v in dir %v, sequence number: %v.\n", accountId, dir, seqnum)

			// TODO: Measure performance impact of this extract resolve request
			// and consider exposing seqnum-based operations in imap.Client.
			uid, err := c.ResolveUid(accountId, dir, seqnum)
			if err != nil {
				Logger.Println("Alert: Reloading message list: failed to download message:", err)
				c.reloadMaillist(accountId, dir)
				return
			}

			count := c.caches[accountId].Dir(dir).MsgsCount()

			if seqnum != uint32(count+1) {
				Logger.Println("Alert: Reloading message list: sequence numbers de-synced.")
				c.reloadMaillist(accountId, dir)
				return
			}
			// If this thing really should go to the end of slice...

			var msg *imap.MessageInfo
			for i := 0; i < *c.GlobalCfg.Connection.MaxTries; i++ {
				msg, err = c.imapConns[accountId].FetchPartialMail(dir, uid, imap.TextOnly)
				if err == nil || !connectionError(err) {
					break
				}
				err = c.connectToServer(accountId)
			}
			if err != nil {
				Logger.Println("Alert: Reloading message list: failed to download message:", err)
				c.reloadMaillist(accountId, dir)
				return
			}

			if err := c.caches[accountId].Dir(dir).AddMsg(msg); err != nil {
				Logger.Println("Cache AddMsg:", err)
			}

			if c.Hooks.ResetDir != nil {
				c.Hooks.ResetDir(accountId, dir)
			}
		},
		MessageRemoved: func(dir string, seqnum uint32) {
			Logger.Printf("Message removed from dir %v on account %v, sequence number: %v.\n", dir, accountId, seqnum)

			count := c.caches[accountId].Dir(dir).MsgsCount()

			if uint32(count) < seqnum {
				Logger.Println("Alert: Reloading message list: sequence number is out of range.")
				c.reloadMaillist(accountId, dir)
				return
			}
			// Look-up UID to remove in cache.
			uid, err := c.caches[accountId].Dir(dir).ResolveUid(seqnum)
			if err != nil {
				Logger.Println("Alert: Reloading message list: failed to resolve UID for removed message.")
				c.reloadMaillist(accountId, dir)
			}
			if err := c.caches[accountId].Dir(dir).DelMsg(uid); err != nil {
				Logger.Println("Cache DelMsg:", err)
			}

			if c.Hooks.ResetDir != nil {
				c.Hooks.ResetDir(accountId, dir)
			}
		},
		MessageUpdate: func(dir string, info *eimap.Message) {
			// TODO
		},
	}
}

func (c *Client) prefetchData(accountId string) error {
	dirs, err := c.GetDirs(accountId)
	if err != nil {
		return err
	}

	for _, dir := range dirs.List() {
		var status *imap.DirStatus
		var err error
		for i := 0; i < *c.GlobalCfg.Connection.MaxTries; i++ {
			status, err = c.imapConns[accountId].Status(dir)
			if err == nil || !connectionError(err) {
				break
			}
			if err := c.connectToServer(accountId); err != nil {
				return err
			}
		}
		if err != nil {
			return err
		}

		cacheVal, err := c.caches[accountId].Dir(dir).UidValidity()
		if cacheVal != status.UidValidity || err == storage.ErrNullValue {
			c.caches[accountId].Dir(dir).InvalidateMsglist()
			c.caches[accountId].Dir(dir).SetUidValidity(status.UidValidity)
		}

	}

	for _, dir := range dirs.List() {
		err = c.prefetchDirData(accountId, dir)
	}

	return err
}

func (c *Client) prefetchDirData(accountId, dir string) error {
	_, err := c.getMsgsList(accountId, dir, true)
	if err != nil {
		return err
	}
	return nil
}

func (c *Client) reloadMaillist(accountId string, dir string) {
	c.getMsgsList(accountId, dir, true)

	if c.Hooks.ResetDir != nil {
		c.Hooks.ResetDir(accountId, dir)
	}
}

// Returns true if passed error is caused by server connection loss and request should be retries.
func connectionError(err error) bool {
	return err.Error() == "imap: connection closed"
}
