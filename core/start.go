package core

import (
	"fmt"
	"log"
	"os"

	eimap "github.com/emersion/go-imap"
	"github.com/foxcpp/mailbox/proto/common"
	"github.com/foxcpp/mailbox/proto/imap"
	"github.com/foxcpp/mailbox/storage"
	deadlock "github.com/sasha-s/go-deadlock"
)

func remove(s []imap.MessageInfo, i int) []imap.MessageInfo {
	return append(s[:i], s[i+1:]...)
}

type accountData struct {
	dirs          StrSet
	unreadCounts  map[string]uint
	messagesByUid map[string]map[uint32]*imap.MessageInfo
	// TODO: Slice should be replaced with linked list because we need to remove items from middle
	// and do it pretty often.
	messagesByDir map[string][]imap.MessageInfo

	uidValidity map[string]uint32

	dirty               bool
	cacheFlusherStopSig chan bool
	lock                deadlock.Mutex
}

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

	Accounts  map[string]storage.AccountCfg
	GlobalCfg storage.GlobalCfg

	caches map[string]*accountData

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

	res.serverCfgs = make(map[string]struct {
		imap, smtp common.ServConfig
	})

	res.Accounts = make(map[string]storage.AccountCfg)
	res.caches = make(map[string]*accountData)
	res.imapConns = make(map[string]*imap.Client)

	for name, info := range accounts {
		Logger.Println("Setting up account", name+"...")
		err := res.AddAccount(name, info, false /* write config */)
		if err != nil {
			res.SkippedAccounts = append(res.SkippedAccounts, *err)
		}
	}

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
	passBytes, err := c.DecryptUsingMaster([]byte(info.Credentials.Pass))
	if err != nil {
		pass = ""
	} else {
		pass = string(passBytes)
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

func (c *Client) initCaches(accountId string) {
	c.caches[accountId] = &accountData{
		dirs:                nil,
		unreadCounts:        make(map[string]uint),
		messagesByUid:       make(map[string]map[uint32]*imap.MessageInfo),
		messagesByDir:       make(map[string][]imap.MessageInfo),
		uidValidity:         make(map[string]uint32),
		dirty:               false,
		cacheFlusherStopSig: make(chan bool),
	}
}

func (c *Client) connectToServer(accountId string) *AccountError {
	var err error

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

			c.caches[accountId].lock.Lock()

			// TODO: Measure performance impact of this extract resolve request
			// and consider exposing seqnum-based operations in imap.Client.
			uid, err := c.ResolveUid(accountId, dir, seqnum)
			if err != nil {
				Logger.Println("Alert: Reloading message list: failed to download message:", err)
				c.caches[accountId].lock.Unlock()
				c.reloadMaillist(accountId, dir)
				return
			}

			msgsByDir := c.caches[accountId].messagesByDir[dir]

			if seqnum != uint32(len(msgsByDir)+1) {
				Logger.Println("Alert: Reloading message list: sequence numbers de-synced.")
				c.caches[accountId].lock.Unlock()
				c.reloadMaillist(accountId, dir)
				return
			}
			// If this thing really should go to the end of slice...

			msg, err := c.imapConns[accountId].FetchPartialMail(dir, uid, imap.TextOnly)
			if err != nil {
				Logger.Println("Alert: Reloading message list: failed to download message:", err)
				c.caches[accountId].lock.Unlock()
				c.reloadMaillist(accountId, dir)
				return
			}
			msgsByDir = append(msgsByDir, *msg)
			c.caches[accountId].messagesByUid[dir][msg.UID] = &msgsByDir[len(msgsByDir)-1]

			c.caches[accountId].dirty = true
			c.caches[accountId].lock.Unlock()

			if c.Hooks.ResetDir != nil {
				c.Hooks.ResetDir(accountId, dir)
			}
		},
		MessageRemoved: func(dir string, seqnum uint32) {
			Logger.Printf("Message removed from dir %v on account %v, sequence number: %v.\n", dir, accountId, seqnum)

			c.caches[accountId].lock.Lock()

			// Look-up UID to remove in cache.
			if uint32(len(c.caches[accountId].messagesByDir[dir])) < seqnum {
				Logger.Println("Alert: Reloading message list: sequence number is out of range.")
				c.caches[accountId].lock.Unlock()
				c.reloadMaillist(accountId, dir)
				return
			}
			uid := c.caches[accountId].messagesByDir[dir][seqnum-1].UID

			c.caches[accountId].messagesByDir[dir] = remove(c.caches[accountId].messagesByDir[dir], int(seqnum-1))
			delete(c.caches[accountId].messagesByUid[dir], uid)

			c.caches[accountId].dirty = true
			c.caches[accountId].lock.Unlock()

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
	Logger.Println("Directories:", dirs.List())

	for _, dir := range c.caches[accountId].dirs.List() {
		value, err := c.imapConns[accountId].UidValidity(dir)
		if err != nil {
			return err
		}
		c.caches[accountId].uidValidity[dir] = value
	}

	for _, dir := range c.caches[accountId].dirs.List() {
		err = c.prefetchDirData(accountId, dir)
	}

	return err
}

func (c *Client) prefetchDirData(accountId, dir string) error {
	list, err := c.getMsgsList(accountId, dir, true)
	if err != nil {
		return err
	}
	Logger.Println(len(list), "messages in", dir)
	return nil
}

func (c *Client) reloadMaillist(accountId string, dir string) {
	c.caches[accountId].lock.Lock()
	c.caches[accountId].messagesByUid[dir] = make(map[uint32]*imap.MessageInfo)
	c.caches[accountId].messagesByDir = make(map[string][]imap.MessageInfo)
	c.caches[accountId].lock.Unlock()
	c.GetMsgsList(accountId, dir)

	if c.Hooks.ResetDir != nil {
		c.Hooks.ResetDir(accountId, dir)
	}
}
