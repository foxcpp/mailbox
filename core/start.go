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

type cache struct {
	dirs          StrSet
	unreadCounts  map[string]uint
	dirsStamp     time.Time
	messagesByUid map[uint32]*imap.MessageInfo
	messagesByDir map[string][]imap.MessageInfo
	msgStamp      map[uint32]time.Time

	lock sync.Mutex
}

type Client struct {
	SkippedAccounts []AccountError

	globalCfg storage.GlobalCfg
	accounts  map[string]storage.AccountCfg
	caches    map[string]*cache

	serverCfgs map[string]struct {
		imap, smtp common.ServConfig
	}

	imapConns map[string]*imap.Client
	smtpConns map[string]*smtp.Client

	watcherStopSignal      chan bool
	cacheCleanerStopSignal chan bool

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
func Launch() (*Client, error) {
	res := new(Client)

	Logger.Println("Loading configuration...")
	globalCfg, err := storage.LoadGlobal()
	if err != nil {
		return nil, err
	}
	res.globalCfg = *globalCfg
	accounts, err := storage.LoadAllAccounts()
	if err != nil {
		return nil, err
	}

	res.serverCfgs = make(map[string]struct {
		imap, smtp common.ServConfig
	})

	res.accounts = make(map[string]storage.AccountCfg)
	res.caches = make(map[string]*cache)
	res.imapConns = make(map[string]*imap.Client)
	res.smtpConns = make(map[string]*smtp.Client)

	for name, info := range accounts {
		Logger.Println("Setting up account", name+"...")
		err := res.AddAccount(name, info, false /* write config */)
		if err != nil {
			res.SkippedAccounts = append(res.SkippedAccounts, *err)
		}
	}

	go res.cacheCleaner()

	return res, nil
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

	info := c.accounts[accountId]

	c.serverCfgs[accountId] = struct{ imap, smtp common.ServConfig }{
		imap: common.ServConfig{
			Host:     info.Server.Imap.Host,
			Port:     info.Server.Imap.Port,
			ConnType: connTypeConv(info.Server.Imap.Encryption),
			User:     info.Credentials.User,
			Pass:     info.Credentials.Pass, // TODO: Decrypt password here.
		},
		smtp: common.ServConfig{
			Host:     info.Server.Smtp.Host,
			Port:     info.Server.Smtp.Port,
			ConnType: connTypeConv(info.Server.Smtp.Encryption),
			User:     info.Credentials.User,
			Pass:     info.Credentials.Pass, // TODO: Decrypt password here.
		},
	}
}

func (c *Client) initCaches(accountId string) {
	// TODO: We can actually write out caches sometime somewhere and read them in here.
	// This way we will be able really speed up things.

	c.caches[accountId] = &cache{
		dirs:          make(StrSet),
		unreadCounts:  make(map[string]uint),
		dirsStamp:     time.Now(),
		messagesByUid: make(map[uint32]*imap.MessageInfo),
		messagesByDir: make(map[string][]imap.MessageInfo),
		msgStamp:      make(map[uint32]time.Time),
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

	Logger.Printf("Connecting to SMTP server (%v:%v)...\n",
		c.serverCfgs[accountId].smtp.Host,
		c.serverCfgs[accountId].smtp.Port)
	c.smtpConns[accountId], err = smtp.Connect(c.serverCfgs[accountId].smtp)
	if err != nil {
		Logger.Println("Connection failed:", err)
		return &AccountError{accountId, err}
	}
	Logger.Println("Authenticating to SMTP server...")
	err = c.smtpConns[accountId].Auth(c.serverCfgs[accountId].smtp)
	if err != nil {
		Logger.Println("Authentication failed:", err)
		return &AccountError{accountId, err}
	}

	return nil
}

func (c *Client) cacheCleaner() {
	Logger.Println("Cache invalidator started.")
	// TODO
}

// flushCaches saves dirs information to cache file on disk to allow quick loading after restart.
func (c *Client) flushCaches() error {
	// TODO: Stub, we don't really have on-disk cache now.
	Logger.Println("Flushing cache...")
	return nil
}

func (c *Client) prefetchData(accountId string) error {
	Logger.Println("Prefetching directories list...")
	var err error
	_, err = c.GetDirs(accountId)
	if err != nil {
		return err
	}
	Logger.Println("Directories:", c.caches[accountId].dirs.List())

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
