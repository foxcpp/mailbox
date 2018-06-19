package core

import (
	"fmt"
	"sync"
	"time"

	"github.com/foxcpp/mailbox/proto/common"
	"github.com/foxcpp/mailbox/proto/imap"
	"github.com/foxcpp/mailbox/proto/smtp"
	"github.com/foxcpp/mailbox/storage"
)

type accountData struct {
	dirs          StrSet
	unreadCounts  map[string]uint
	dirsStamp     time.Time
	messagesByUid map[uint32]*imap.MessageInfo
	messagesByDir map[string][]imap.MessageInfo
	msgStamp      map[uint32]time.Time

	updatesWatcherSig chan bool
	cacheCleanerSig   chan bool

	lock sync.Mutex
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
	smtpConns map[string]*smtp.Client

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
	res.smtpConns = make(map[string]*smtp.Client)

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

	pass := info.Credentials.Pass // TODO: Decryption should occur here.
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
	// TODO: We can actually write out caches sometime somewhere and read them in here.
	// This way we will be able really speed up things.

	c.caches[accountId] = &accountData{
		dirs:              nil,
		unreadCounts:      make(map[string]uint),
		dirsStamp:         time.Now(),
		messagesByUid:     make(map[uint32]*imap.MessageInfo),
		messagesByDir:     make(map[string][]imap.MessageInfo),
		msgStamp:          make(map[uint32]time.Time),
		updatesWatcherSig: make(chan bool),
		cacheCleanerSig:   make(chan bool),
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

	return nil
}

func (c *Client) updatesWatcher(accountId string) {
	select {
	case <-c.caches[accountId].updatesWatcherSig:
		return
	case <-c.imapConns[accountId].RawClient().Updates:
		return
	}
}

func (c *Client) cacheCleaner(accountId string) {
	Logger.Println("Cache invalidator started.")
}

// flushCaches saves dirs information to cache file on disk to allow quick loading after restart.
func (c *Client) flushCaches() error {
	// TODO: Stub, we don't really have on-disk cache now.
	Logger.Println("Flushing cache...")
	return nil
}

func (c *Client) prefetchData(accountId string) error {
	Logger.Println("Prefetching directories list...")
	dirs, err := c.GetDirs(accountId)
	if err != nil {
		return err
	}
	Logger.Println("Directories:", dirs.List())

	Logger.Println("Prefetching directories status...")
	// Even though we ignore returned values - caches will
	// be populated with needed data.
	for _, dir := range c.caches[accountId].dirs.List() {
		Logger.Println("Looking into", dir+"...")
		count, err := c.GetUnreadCount(accountId, dir)
		if err != nil {
			return err
		}
		Logger.Println(count, "unread messages in", dir)
	}

	Logger.Println("Prefetching INBOX contents...")
	// User will very likely first open INBOX, right?
	list, err := c.GetMsgsList(accountId, "INBOX")
	Logger.Println(len(list), "messages in INBOX")
	return err
}
