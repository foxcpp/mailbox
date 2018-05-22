package smtp

import (
	"crypto/tls"
	"errors"
	"net"
	"net/smtp"
	"strconv"
	"strings"

	sasl "github.com/emersion/go-sasl"
	"github.com/foxcpp/gopher-mail/common"
)

type Client smtp.Client

func tlsHandshake(conn net.Conn, hostname string) (*smtp.Client, error) {
	return smtp.NewClient(tls.Client(conn, &tls.Config{ServerName: hostname}), hostname)
}

func starttlsHandshake(conn net.Conn, hostname string) (*smtp.Client, error) {
	conf := &tls.Config{ServerName: hostname}
	c, err := smtp.NewClient(conn, hostname)
	if err != nil {
		return nil, err
	}

	if ok, _ := c.Extension("STARTTLS"); !ok {
		return nil, errors.New("starttls: not supported")
	}

	if err := c.StartTLS(conf); err != nil {
		return nil, err
	} else {
		return c, nil
	}
}

// Connect connects to server using specified configuration.
func Connect(target common.ServConfig) (*Client, error) {
	addr := target.Host + ":" + strconv.Itoa(int(target.Port))

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}

	var c *smtp.Client
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

	return (*Client)(c), nil
}

// Auth authenticates using specified configuration if possible.
func (c *Client) Auth(conf common.ServConfig) error {
	if ok, kinds := c.Extension("AUTH"); ok {
		if strings.Contains(kinds, "PLAIN") && conf.User != "" {
			if err := c.Auth(sasl.NewPlainClient("", target.User, target.Pass)); err != nil {
				return nil, err
			}
		} else if strings.Contains(kinds, "ANONYMOUS") {
			if err := c.Auth(sasl.NewAnonymousClient("See_IP")); err != nil {
				return nil, err
			}
		} else {
			return errors.New("auth: not supported")
		}
	} else {
		return errors.New("auth: not supported")
	}
}

func (c *Client) Close() error {
	return (*smtp.Client)(c).Quit()
}
