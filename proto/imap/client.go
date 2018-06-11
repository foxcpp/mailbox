package imap

import (
	"crypto/tls"
	"errors"
	"net"
	"strconv"

	eimap "github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	sasl "github.com/emersion/go-sasl"
	"github.com/foxcpp/mailbox/proto/common"
)

type Client struct {
	cl            *client.Client
	seenMailboxes map[string]eimap.MailboxInfo
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

func Connect(target common.ServConfig) (*Client, error) {
	addr := target.Host + ":" + strconv.Itoa(int(target.Port))

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}

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

	return &Client{cl: c}, nil
}

func (c *Client) Auth(conf common.ServConfig) error {
	return c.cl.Authenticate(sasl.NewPlainClient("", conf.User, conf.Pass))
}

func (c *Client) Close() error {
	c.cl.Close()
	c.cl.Logout()
	return c.cl.Terminate()
}

func (c *Client) Logout() error {
	return c.cl.Logout()
}
