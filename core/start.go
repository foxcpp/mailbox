// core package bundles all lower levers into one solid system. It also contains most of
// the mailbox client logic and presents interface to upper level (fronend).
package core

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"

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

	// Per-account list of directories message list for which is downloaded early (during Launch).
	// Currently this is only INBOX.
	prefetchDirs map[string][]string

	imapDirSep sync.Map

	logger, debugLog *log.Logger
	logFile          *os.File
}

type AccountError struct {
	AccountId string
	Err       error
}

func (e AccountError) Error() string {
	return fmt.Sprintf("accountErr %v: %v", e.AccountId, e.Err)
}

// Launch reads client configuration and connects to servers.
//
// hooks argument provides set of callbacks to call on various events. All functions must be provided.
// userLogOut is where log messages meant for user should be written.
func Launch(hooks FrontendHooks, userLogOut io.Writer) (*Client, error) {
	res := new(Client)
	res.Hooks = hooks

	logFile, err := os.OpenFile(filepath.Join(storage.GetDirectory(), "log.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		// Not critical, ignore.
		res.logger = log.New(userLogOut, "", log.LstdFlags)
		res.debugLog = log.New(userLogOut, "core[debug]: ", log.LstdFlags)
		res.debugLog.Println("Failed to open log file. Is everything fine with permissions?", err)
	} else {
		res.logFile = logFile
		res.logger = log.New(io.MultiWriter(userLogOut, logFile), "", log.LstdFlags)
		res.debugLog = log.New(logFile, "core[debug]: ", log.LstdFlags)
	}

	res.logger.Println("Loading configuration...")
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
	res.prefetchDirs = make(map[string][]string)

	for name, info := range accounts {
		res.debugLog.Println("Setting up account", name+"...")
		err := res.LoadAccount(name, info)
		if err != nil {
			res.SkippedAccounts = append(res.SkippedAccounts, *err)
		}
	}

	// Save new config, in case something changed something.
	storage.SaveGlobal(&res.GlobalCfg)

	return res, nil
}

func (c *Client) Stop() {
	for name := range c.Accounts {
		c.UnloadAccount(name)
	}
	c.logFile.Close()
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

	c.prefetchDirs[accountId] = []string{"INBOX"}
}

func (c *Client) connectToServer(accountId string) *AccountError {
	var err error

	if c.imapConns[accountId] != nil {
		if err := c.imapConns[accountId].Reconnect(); err != nil {
			return &AccountError{accountId, err}
		}
		if err := c.imapConns[accountId].Auth(c.serverCfgs[accountId].imap); err != nil {
			return &AccountError{accountId, err}
		}
		return nil
	}

	c.logger.Printf("Connecting to IMAP server (%v:%v)...\n",
		c.serverCfgs[accountId].imap.Host,
		c.serverCfgs[accountId].imap.Port)
	c.imapConns[accountId], err = imap.Connect(c.serverCfgs[accountId].imap)
	if err != nil {
		c.logger.Println("Connection failed:", err)
		return &AccountError{accountId, err}
	}
	c.logger.Println("Authenticating to IMAP server...")
	err = c.imapConns[accountId].Auth(c.serverCfgs[accountId].imap)
	if err != nil {
		c.logger.Println("Authentication failed:", err)
		return &AccountError{accountId, err}
	}

	c.imapConns[accountId].Callbacks = c.makeUpdateCallbacks(accountId)
	c.imapConns[accountId].Logger = *log.New(c.logFile, "imap["+accountId+",debug] ", log.LstdFlags)

	return nil
}

func (c *Client) dirSep(accountId string) string {
	val, ok := c.imapDirSep.Load(accountId)
	if !ok {
		panic("Trying to get directory level separator before it's known.")
	}
	return val.(string)
}

func (c *Client) makeUpdateCallbacks(accountId string) *imap.UpdateCallbacks {
	return &imap.UpdateCallbacks{
		NewMessage: func(dir string, seqnum uint32) {
			c.logger.Printf("New message for account %v in dir %v.\n", accountId, dir)
			c.debugLog.Printf("New message for account %v in dir %v, sequence number: %v.\n", accountId, dir, seqnum)

			rawDir := dir
			dir = c.normalizeDirName(accountId, dir)

			// TODO: Measure performance impact of this extract resolve request
			// and consider exposing seqnum-based operations in imap.Client.
			uid, err := c.resolveUid(accountId, dir, seqnum)
			if err != nil {
				c.debugLog.Println("Alert: Reloading message list: failed to download message:", err)
				c.reloadMaillist(accountId, dir)
				return
			}

			count := c.caches[accountId].Dir(dir).MsgsCount()

			if seqnum != uint32(count+1) {
				c.debugLog.Println("Alert: Reloading message list: sequence numbers de-synced.")
				c.reloadMaillist(accountId, dir)
				return
			}
			// If this thing really should go to the end of slice...

			var msg *imap.MessageInfo
			for i := 0; i < *c.GlobalCfg.Connection.MaxTries; i++ {
				msg, err = c.imapConns[accountId].FetchPartialMail(rawDir, uid, imap.TextOnly)
				if err == nil || !connectionError(err) {
					break
				}
				err = c.connectToServer(accountId)
			}
			if err != nil {
				c.debugLog.Println("Alert: Reloading message list: failed to download message:", err)
				c.reloadMaillist(accountId, dir)
				return
			}

			if err := c.caches[accountId].Dir(dir).AddMsg(msg); err != nil {
				c.debugLog.Println("Cache AddMsg:", err)
			}

			if c.Hooks.ResetDir != nil {
				c.Hooks.ResetDir(accountId, dir)
			}
		},
		MessageRemoved: func(dir string, seqnum uint32) {
			c.logger.Printf("Message removed from dir %v on account %v.\n", dir, accountId)
			c.debugLog.Printf("Message removed from dir %v on account %v, sequence number: %v.\n", dir, accountId, seqnum)

			dir = c.normalizeDirName(accountId, dir)

			count := c.caches[accountId].Dir(dir).MsgsCount()

			if uint32(count) < seqnum {
				c.debugLog.Println("Alert: Reloading message list: sequence number is out of range.")
				c.reloadMaillist(accountId, dir)
				return
			}
			// Look-up UID to remove in cache.
			uid, err := c.caches[accountId].Dir(dir).ResolveUid(seqnum)
			if err != nil {
				c.debugLog.Println("Alert: Reloading message list: failed to resolve UID for removed message.")
				c.reloadMaillist(accountId, dir)
			}
			if err := c.caches[accountId].Dir(dir).DelMsg(uid); err != nil {
				c.debugLog.Println("Cache DelMsg:", err)
			}

			if c.Hooks.ResetDir != nil {
				c.Hooks.ResetDir(accountId, dir)
			}
		},
		MessageUpdate: func(dir string, info *eimap.Message) {
			// Basically, this is only Flags change.
			if info.Uid != 0 && info.Flags != nil {
				c.caches[accountId].Dir(c.normalizeDirName(accountId, dir)).ReplaceTagList(info.Uid, info.Flags)
			}
		},
		MboxUpdate: func(status *eimap.MailboxStatus) {
			dir := c.normalizeDirName(accountId, status.Name)

			uidv, err := c.caches[accountId].Dir(dir).UidValidity()
			if err != nil {
				return
			}
			if uidv != status.UidValidity {
				c.debugLog.Println("UIDVALIDITY changed for dir", status.Name)
				c.reloadMaillist(accountId, status.Name)
				c.caches[accountId].Dir(dir).SetUidValidity(uidv)
			}
			c.caches[accountId].Dir(dir).SetUnreadCount(uint(status.Unseen))
		},
	}
}

// prefetchData is called for early data population shortly after
// account loading.
func (c *Client) prefetchData(accountId string) error {
	_, err := c.GetDirs(accountId, true)
	if err != nil {
		return err
	}

	for _, dir := range c.prefetchDirs[accountId] {
		var status *imap.DirStatus
		var err error
		for i := 0; i < *c.GlobalCfg.Connection.MaxTries; i++ {
			status, err = c.imapConns[accountId].Status(c.rawDirName(accountId, dir))
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
			if cacheVal != status.UidValidity {
				c.debugLog.Println("UIDVALIDITY changed")
			}
			c.caches[accountId].Dir(dir).InvalidateMsglist()
			c.caches[accountId].Dir(dir).SetUidValidity(status.UidValidity)
		}

		c.getMsgsList(accountId, dir, true)
	}

	return err
}

func (c *Client) reloadMaillist(accountId string, dir string) {
	c.getMsgsList(accountId, dir, true)

	if c.Hooks.ResetDir != nil {
		c.Hooks.ResetDir(accountId, dir)
	}
}

// Returns true if passed error is caused by server connection loss and request should be retries.
func connectionError(err error) bool {
	return err.Error() == "imap: connection closed" || err.Error() == "short write"
}
