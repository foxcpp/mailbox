package common

import (
	"fmt"
	"io"
	"net/mail"
	"strings"
	"time"

	message "github.com/emersion/go-message"
)

func MarshalAddressList(in []mail.Address) string {
	addrStrs := []string{}
	for _, addr := range in {
		addrStrs = append(addrStrs, MarshalAddress(addr))
	}
	return strings.Join(addrStrs, ", ")
}

func MarshalAddress(addr mail.Address) string {
	return strings.TrimSpace(fmt.Sprintf("%v <%v>", addr.Name, addr.Address))
}

func MarshalDate(d time.Time) string {
	return d.Format("Mon, 2 Jan 2006 15:04:05 -0700")
}

// Write writes out message headers + body in format suitable for
// transmission using SMTP protocol.
//
// Transfer encoding is picked depending on "nature" of the body of each part.
// If part contains only ASCII printabe characters it will be written as is, if
// few bytes (<25%) with 8 bit set then QP encoding will be used,
// otherwise Base-64 will be used.
func (m *Msg) Write(out io.Writer) error {
	allHdrs := message.Header{}
	if len(m.From.Address) != 0 {
		allHdrs.Set("From", MarshalAddress(m.From))
	}
	if len(m.To) != 0 {
		allHdrs.Set("To", MarshalAddressList(m.To))
	}
	if len(m.Subject) != 0 {
		allHdrs.Set("Subject", m.Subject)
	}
	if len(m.Cc) != 0 {
		allHdrs.Set("Cc", MarshalAddressList(m.Cc))
	}
	if len(m.Bcc) != 0 {
		allHdrs.Set("Bcc", MarshalAddressList(m.Bcc))
	}
	if len(m.ReplyTo.Address) != 0 {
		allHdrs.Set("Reply-To", MarshalAddress(m.ReplyTo))
	}
	if !m.Date.IsZero() {
		allHdrs.Set("Date", MarshalDate(m.Date))
	}
	allHdrs.Set("MIME-Version", "1.0")
	for k, v := range m.Misc {
		allHdrs[k] = v
	}

	if len(m.Parts) == 0 {
		w, err := message.CreateWriter(out, allHdrs)
		if err != nil {
			return err
		}
		return w.Close()
	}

	var err error
	if len(m.Parts) == 1 {
		err = writeRegular(m, allHdrs, out)
	} else {
		err = writeMultipart(m, allHdrs, out)
	}
	return err
}

func writeRegular(m *Msg, hdrs message.Header, out io.Writer) error {
	if _, prs := hdrs["Content-Type"]; !prs {
		hdrs.SetContentType("text/plain", map[string]string{"charset": "utf-8"})
	}
	hdrs.Set("Content-Transfer-Encoding", pickEncoding(m.Parts[0].Body))

	w, err := message.CreateWriter(out, hdrs)
	if err != nil {
		return err
	}
	w.Write(m.Parts[0].Body)
	return w.Close()
}

func writeMultipart(m *Msg, hdrs message.Header, out io.Writer) error {
	boundary := randomBoundary()
	if _, prs := hdrs["Content-Type"]; !prs {
		hdrs.SetContentType("multipart/mixed", map[string]string{"boundary": boundary})
	}
	w, err := message.CreateWriter(out, hdrs)
	if err != nil {
		return err
	}

	for _, part := range m.Parts {
		partHdrs := message.Header{}
		if part.Type.Value != "" {
			partHdrs.Set("Content-Type", part.Type.String())
		}
		if part.Disposition.Value != "" {
			partHdrs.Set("Content-Disposition", part.Type.String())
		}
		for k, v := range part.Misc {
			partHdrs[k] = v
		}

		partHdrs.Set("Content-Transfer-Encoding", pickEncoding(part.Body))
		if _, prs := partHdrs["Content-Type"]; !prs {
			partHdrs.SetContentType("text/plain", map[string]string{"charset": "utf-8"})
		}
		pw, err := w.CreatePart(partHdrs)
		if err != nil {
			return err
		}
		pw.Write(part.Body)
		if err := pw.Close(); err != nil {
			return err
		}
	}
	return w.Close()
}

func randomBoundary() string {
	return RandomStr(64)
}

func pickEncoding(body []byte) string {
	ascii := 0
	for _, b := range body {
		if b < 126 && (b >= 32 /* space */ || b == 10 /* LF */ || b == 13 /* CR */) {
			ascii += 1
		}
	}

	if ascii == len(body) {
		return "7bit"
	}
	if float64(ascii)/float64(len(body)) > 0.75 {
		return "quoted-printable"
	}
	return "base64"
}
