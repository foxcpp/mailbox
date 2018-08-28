package common

import (
	"io"
	"io/ioutil"
	"net/mail"

	message "github.com/emersion/go-message"
)

func ReadMsg(in io.Reader) (*Msg, error) {
	res := new(Msg)

	m, err := message.Read(in)
	if err != nil {
		return nil, err
	}

	if err := readHeaders(res, m); err != nil {
		return nil, err
	}
	if err := readBody(res, m); err != nil {
		return nil, err
	}

	return res, nil
}

func readHeaders(res *Msg, m *message.Entity) error {
	res.Subject = m.Header.Get("Subject")
	res.Date, _ = mail.ParseDate(m.Header.Get("Date"))

	from, _ := mail.ParseAddress(m.Header.Get("From"))
	if from != nil {
		res.From = *from
	}
	replyTo, _ := mail.ParseAddress(m.Header.Get("Reply-To"))
	if replyTo != nil {
		res.ReplyTo = *replyTo
	}
	res.To, _ = ConvertAddrList(mail.ParseAddressList(m.Header.Get("To")))
	res.Cc, _ = ConvertAddrList(mail.ParseAddressList(m.Header.Get("Cc")))
	res.Bcc, _ = ConvertAddrList(mail.ParseAddressList(m.Header.Get("Bcc")))

	delete(m.Header, "Date")
	delete(m.Header, "Subject")
	delete(m.Header, "From")
	delete(m.Header, "Reply-To")
	delete(m.Header, "To")
	delete(m.Header, "Cc")
	delete(m.Header, "Bcc")
	res.Misc = m.Header
	return nil
}

func readBody(res *Msg, m *message.Entity) error {
	if mr := m.MultipartReader(); mr != nil {
		// Multipart message.
		for {
			outPart := Part{}
			part, err := mr.NextPart()
			if err != nil {
				if err == io.EOF {
					break
				}
				return err
			}

			outPart.Type.Value, outPart.Type.Params, err = part.Header.ContentType()
			outPart.Body, err = ioutil.ReadAll(part.Body)
			if err != nil {
				return err
			}
			outPart.Size = uint32(len(outPart.Body))
			part.Header.Del("Content-Type")
			outPart.Misc = part.Header

			res.Parts = append(res.Parts, outPart)
		}
	} else {
		// Regular message.
		outPart := Part{}
		var err error
		outPart.Type.Value, outPart.Type.Params, err = m.Header.ContentType()
		outPart.Body, err = ioutil.ReadAll(m.Body)
		if err != nil {
			return err
		}
		outPart.Size = uint32(len(outPart.Body))

		res.Parts = append(res.Parts, outPart)
	}
	return nil
}
