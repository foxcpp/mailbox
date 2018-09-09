package imap

import (
	"crypto/tls"
	"errors"
	"log"
	"net"
	"strconv"
	"sync"
	"time"

	eimap "github.com/emersion/go-imap"
	"github.com/emersion/go-imap-idle"
	"github.com/emersion/go-imap-move"
	"github.com/emersion/go-imap-uidplus"
	"github.com/emersion/go-imap/client"
	"github.com/emersion/go-sasl"
	"github.com/foxcpp/mailbox/proto/common"
)

/*
	Most callbacks should manually resolve sequence number into UID if
	necessary, client provides wrapper for this purpose: ResolveSeqNum.
	Also, newInfo contains partial information in special form, so callback
	should do MessageInfo.UpdateFrom(newInfo) to update cached information (if any).

	Callbacks for dirs other than INBOX usually called only during operations
	on these directories because we monitor only INBOX. Thus notifications
	about INBOX can be delivered at any time.

	Note: Callback MUST NOT ignore any calls, because sequence numbers depend
	on each other and should be re-synced on each opeartion.

	Note: Usually callbacks will be called from separate goroutine so code
	should be thread-safe.
*/
type UpdateCallbacks struct {
	NewMessage     func(dir string, seqnum uint32)
	MessageUpdate  func(dir string, newInfo *eimap.Message)
	MessageRemoved func(dir string, seqnum uint32)
}

type Client struct {
	Callbacks         *UpdateCallbacks
	KnownMailboxSizes map[string]uint32
	Logger            log.Logger

	maxUploadSize uint32

	seenMailboxes map[string]eimap.MailboxInfo

	updates               chan client.Update
	updatesDispatcherStop chan bool

	idlerInterrupt chan bool

	IOLock sync.Mutex
	cl     *client.Client

	uidplus *uidplus.Client
	move    *move.Client
	idle    *idle.IdleClient
}

func tlsHandshake(conn net.Conn, hostname string) (*client.Client, error) {
	return client.New(tls.Client(conn, &tls.Config{ServerName: hostname}))
}

func starttlsHandshake(conn net.Conn, hostname string) (*client.Client, error) {
	conf := &tls.Config{ServerName: hostname}
	c, err := client.New(conn)
	if err != nil {
		return nil, err
	}

	caps, err := c.Capability()
	if err != nil {
		return nil, err
	}
	if _, prs := caps["STARTTLS"]; !prs {
		return nil, errors.New("starttls: not supported")
	}

	if err := c.StartTLS(conf); err != nil {
		return nil, err
	}
	return c, nil
}

func connect(target common.ServConfig) (*client.Client, error) {
	addr := target.Host + ":" + strconv.Itoa(int(target.Port))

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}

	// Connection must complete in 30 seconds.
	conn.SetDeadline(time.Now().Add(30 * time.Second))

	var c *client.Client
	if target.ConnType == common.TLS {
		var err error
		c, err = tlsHandshake(conn, target.Host)
		if err != nil {
			return nil, err
		}
	} else if target.ConnType == common.STARTTLS {
		var err error
		c, err = starttlsHandshake(conn, target.Host)
		if err != nil {
			return nil, err
		}
	}

	// Reset deadline.
	conn.SetDeadline(time.Time{})

	// 30 timeout for any I/O.
	c.Timeout = 30 * time.Second
	return c, nil
}

func Connect(target common.ServConfig) (*Client, error) {
	c, err := connect(target)
	if err != nil {
		return nil, err
	}

	res := &Client{cl: c}
	// We have that small buffer to prevent updates queue from being filled
	// with updates from different mailboxes, as this will break a lot of things.
	res.updates = make(chan client.Update, 16)
	res.idlerInterrupt = make(chan bool)
	res.updatesDispatcherStop = make(chan bool)
	res.KnownMailboxSizes = make(map[string]uint32)
	res.cl.Updates = res.updates

	res.idle = idle.NewClient(res.cl)
	res.move = move.NewClient(res.cl)
	res.uidplus = uidplus.NewClient(res.cl)

	//res.cl.SetDebug(os.Stderr)

	go res.updatesWatch()

	return res, nil
}

func (c *Client) RawClient() *client.Client {
	// TODO: This should be refactored into exported variable.
	return c.cl
}

func (c *Client) Auth(conf common.ServConfig) error {
	c.IOLock.Lock()
	defer c.IOLock.Unlock()

	err := c.cl.Authenticate(sasl.NewPlainClient("", conf.User, conf.Pass))
	if err == nil {
		go c.idleOnInbox()
	}
	return err
}

// Reconnectrecovers lost connection.
// Note: If this function fails connection will be left in closed ("null") state.
func (c *Client) Reconnect(target common.ServConfig) error {
	// Exactly that order to prevent deadlock (IDLE goroutine locks IOLock so we need to stop it before locking).
	c.updatesDispatcherStop <- true
	<-c.updatesDispatcherStop
	c.idlerInterrupt <- true
	c.IOLock.Lock()
	<-c.idlerInterrupt
	defer c.IOLock.Unlock()

	c.cl = nil
	var err error
	c.cl, err = connect(target)
	c.idle = idle.NewClient(c.cl)
	c.move = move.NewClient(c.cl)
	c.uidplus = uidplus.NewClient(c.cl)
	return err
}

func (c *Client) Close() error {
	c.updatesDispatcherStop <- true
	<-c.updatesDispatcherStop

	c.idlerInterrupt <- true
	<-c.idlerInterrupt

	c.cl.Logout()
	return c.cl.Terminate()
}

func (c *Client) Logout() error {
	c.IOLock.Lock()
	defer c.IOLock.Unlock()
	return c.cl.Logout()
}
