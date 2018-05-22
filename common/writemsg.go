package common

import (
	"encoding/base64"
	"fmt"
	"io"
	"mime/multipart"
	"net/mail"
	"net/textproto"
	"strings"
	"time"
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
	// TODO: "Defragment" code, this crap is unreadable! Multipart/regular
	// messages processing is intermixed.
	allHdrs := mail.Header{}
	if len(m.From.Address) != 0 {
		allHdrs["From"] = []string{MarshalAddress(m.From)}
	}
	if len(m.To) != 0 {
		allHdrs["To"] = []string{MarshalAddressList(m.To)}
	}
	if len(m.Subject) != 0 {
		allHdrs["Subject"] = []string{m.Subject}
	}
	if len(m.Cc) != 0 {
		allHdrs["Cc"] = []string{MarshalAddressList(m.Cc)}
	}
	if len(m.Bcc) != 0 {
		allHdrs["Bcc"] = []string{MarshalAddressList(m.Bcc)}
	}
	if len(m.ReplyTo.Address) != 0 {
		allHdrs["Reply-To"] = []string{MarshalAddress(m.ReplyTo)}
	}
	if !m.Date.IsZero() {
		allHdrs["Date"] = []string{MarshalDate(m.Date)}
	}
	allHdrs["MIME-Version"] = []string{"1.0"}
	// Used with multipart messages.
	var boundary string
	if _, prs := allHdrs["Content-Type"]; !prs {
		if len(m.Parts) == 1 {
			allHdrs["Content-Type"] = []string{"text/plain; charset=utf-8"}
		} else {
			boundary = randomBoundary()
			allHdrs["Content-Type"] = []string{"multipart/mixed; boundary=" + boundary}
		}
	}
	var enc Encoding
	if len(m.Parts) == 1 {
		// writeRegular will use the same encoding because pickEncoding is
		// determistic.
		var name string
		name, enc = pickEncoding(m.Parts[0].Body)
		allHdrs["Content-Transfer-Encoding"] = []string{name}
	}
	if err := writeHeaders(allHdrs, out); err != nil {
		return err
	}

	var err error
	if len(m.Parts) > 1 {
		err = writeMultipart(m, boundary, out)
	} else {
		encBody := make([]byte, enc.EncodedLen(len(m.Parts[0].Body)))
		enc.Encode(encBody, m.Parts[0].Body)
		_, err = out.Write(encBody)
	}
	if err != nil {
		return err
	}

	return nil
}

func writeMultipart(m *Msg, boundary string, out io.Writer) error {
	outWrap := multipart.NewWriter(out)
	outWrap.SetBoundary(boundary)

	for _, part := range m.Parts {
		allHdrs := mail.Header{}
		allHdrs["Content-Type"] = []string{part.Type.String()}
		encName, enc := pickEncoding(part.Body)
		allHdrs["Content-Transfer-Encoding"] = []string{encName}
		for k, v := range part.Misc {
			allHdrs[k] = v
		}

		partW, err := outWrap.CreatePart(textproto.MIMEHeader(allHdrs))
		if err != nil {
			return err
		}

		encBody := make([]byte, enc.EncodedLen(len(part.Body)))
		enc.Encode(encBody, part.Body)
		_, err = partW.Write(encBody)
		if err != nil {
			return err
		}
	}

	// One of rare cases where Close can actually fail and we should not ignore
	// this failure.
	return outWrap.Close()
}

func randomBoundary() string {
	return RandomStr(64)
}

func writeHeaders(hdrs mail.Header, out io.Writer) error {
	for k, v := range hdrs {
		// TODO: Use QP encoding for headers (?)
		_, err := out.Write([]byte(fmt.Sprintf("%v: %v\r\n", k, strings.Join(v, " "))))
		if err != nil {
			return err
		}
	}
	if _, err := out.Write([]byte("\r\n")); err != nil {
		return err
	}
	return nil
}

func pickEncoding(body []byte) (string, Encoding) {
	ascii := 0
	for _, b := range body {
		if b < 126 && (b >= 32 /* space */ || b == 10 /* LF */ || b == 13 /* CR */) {
			ascii += 1
		}
	}

	if ascii == len(body) {
		return "7bit", DummyEncoding{}
	}
	// TODO: Not implemented yet.
	//if ascii/len(body) < 0.75 {
	//	return "quoted-printed", qpEncoding
	//}
	return "base64", base64.StdEncoding
}
