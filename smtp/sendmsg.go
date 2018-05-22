package smtp

import (
	"github.com/emersion/go-smtp"
	"github.com/foxcpp/gopher-mail/common"
)

func (c *Client) Send(msg common.Msg) error {
	cl := (*smtp.Client)(c)
	if err := cl.Mail(msg.From.Address); err != nil {
		return err
	}

	for _, to := range msg.To {
		if err := cl.Rcpt(to.Address); err != nil {
			if err := cl.Reset(); err != nil {
				return err
			}
			return err
		}
	}

	w, err := cl.Data()
	if err != nil {
		if err := cl.Reset(); err != nil {
			return err
		}
		return err
	}

	if err := msg.Write(w); err != nil {
		return err
	}

	if err := w.Close(); err != nil {
		return err
	}

	return nil
}
