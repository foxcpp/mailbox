package common

import (
	"encoding/base64"
	"errors"
	"io"
	"io/ioutil"
	"mime"
	"mime/multipart"
	"net/mail"
	"strings"
)

var ErrTruncated = errors.New("message is too large to read fully, some parts may be missing")

func ReadMsg(in io.Reader) (*Msg, error) {
	res := new(Msg)

	msg, err := mail.ReadMessage(in)
	if err != nil {
		return nil, err
	}

	if err := readHeaders(res, msg); err != nil {
		return nil, err
	}
	if err := readBody(res, msg); err != nil {
		return nil, err
	}

	return res, nil
}

func readHeaders(out *Msg, in *mail.Message) error {
	convAddrList := func(in []*mail.Address, err error) ([]mail.Address, error) {
		if err != nil {
			return nil, err
		}

		res := make([]mail.Address, 0)
		for _, a := range in {
			res = append(res, *a)
		}
		return res, nil
	}

	out.Date, _ = mail.ParseDate(in.Header.Get("Date"))
	out.Subject = in.Header.Get("Subject")
	from, _ := mail.ParseAddress(in.Header.Get("From"))
	if from != nil {
		out.From = *from
	}
	replyTo, _ := mail.ParseAddress(in.Header.Get("Reply-To"))
	if replyTo != nil {
		out.ReplyTo = *replyTo
	}
	out.To, _ = convAddrList(mail.ParseAddressList(in.Header.Get("To")))
	out.Cc, _ = convAddrList(mail.ParseAddressList(in.Header.Get("Cc")))
	out.Bcc, _ = convAddrList(mail.ParseAddressList(in.Header.Get("Bcc")))
	delete(in.Header, "Date")
	delete(in.Header, "Subject")
	delete(in.Header, "From")
	delete(in.Header, "Reply-To")
	delete(in.Header, "To")
	delete(in.Header, "Cc")
	delete(in.Header, "Bcc")
	out.Misc = in.Header
	return nil
}

func readBody(out *Msg, in *mail.Message) error {
	t, params, err := mime.ParseMediaType(in.Header.Get("Content-Type"))
	if err != nil {
		// Default RFC 822 messages are typed by this protocol as plain
		// text in the US-ASCII character set, which can be explicitly
		// specified as "Content-type: text/plain; charset=us-ascii". If no
		// Content-Type is specified, either by error or by an older user
		// agent, this default is assumed.
		// -- RFC 1341, Section 4 (The Content-Type Header Field)
		// Instead, we assume UTF-8 because we are following Postel's law.
		t = "text/plain"
		params = map[string]string{"charset": "UTF-8"}
	}
	if strings.HasPrefix(t, "multipart/") {
		return readMultipart(out, in, params)
	} else {
		return readRegularBody(t, params, out, in)
	}
}

func readMultipart(out *Msg, in *mail.Message, params map[string]string) error {
	boundary, prs := params["boundary"]
	if !prs {
		return errors.New("multipart: no boundary")
	}

	multipart := multipart.NewReader(in.Body, boundary)

	for {
		part, err := multipart.NextPart()
		if err != nil {
			if err == io.EOF {
				break
			} else {
				return err
			}
		}
		defer part.Close()

		res := Part{}
		if t, params, err := mime.ParseMediaType(part.Header.Get("Content-Type")); err != nil {
			res.Type.T = "text/plain"
			res.Type.Params = map[string]string{"charset": "UTF-8"}
		} else {
			res.Type.T = t
			res.Type.Params = params
		}

		// TODO: There should be way to limit reads to prevent DoS attacks.
		rawBody, err := ioutil.ReadAll(part)
		if err != nil {
			return err
		}

		// Decode encoded body if any.
		//  quoted-printable is already transparently covered by multipart parsing utilities.
		//  7bit, 8bit, binary don't make a sense for us.
		//  base64 is handled below.
		if strings.ToLower(part.Header.Get("Content-Transfer-Encoding")) == "base64" {
			res.Body = make([]byte, base64.StdEncoding.DecodedLen(len(rawBody)))
			_, err := base64.StdEncoding.Decode(res.Body, rawBody)
			if err != nil {
				return err
			}
		} else {
			res.Body = rawBody
		}

		// Consume already-used-fields.
		delete(part.Header, "Content-Transfer-Encoding")
		delete(part.Header, "Content-Type")
		// ...and copy rest to the Misc map.
		res.Misc = mail.Header{}
		for k, v := range part.Header {
			res.Misc[k] = v
		}

		out.Parts = append(out.Parts, res)
	}
	return nil
}

func readRegularBody(type_ string, typeParams map[string]string, out *Msg, in *mail.Message) error {
	res := Part{}
	if t, params, err := mime.ParseMediaType(in.Header.Get("Content-Type")); err != nil {
		res.Type.T = "text/plain"
		res.Type.Params = map[string]string{"charset": "UTF-8"}
	} else {
		res.Type.T = t
		res.Type.Params = params
	}

	// TODO: There should be way to limit reads to prevent DoS attacks.
	rawBody, err := ioutil.ReadAll(in.Body)
	if err != nil {
		return err
	}

	// Decode encoded body if any.
	//  7bit, 8bit, binary don't make a sense for us.
	//  base64 and quoted-printable are handled below. TODO!
	if strings.ToLower(in.Header.Get("Content-Transfer-Encoding")) == "base64" {
		res.Body = make([]byte, base64.StdEncoding.DecodedLen(len(rawBody)))
		_, err := base64.StdEncoding.Decode(res.Body, rawBody)
		if err != nil {
			return err
		}
	} else if strings.ToLower(in.Header.Get("Content-Transfer-Encoding")) == "quoted-printable" {
		return errors.New("quoted-printable: not implemented")
	} else {
		res.Body = rawBody
	}

	// Consume already-used-fields.
	delete(in.Header, "Content-Transfer-Encoding")
	delete(in.Header, "Content-Type")

	out.Parts = []Part{res}
	return nil
}
