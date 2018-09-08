package common

import (
	"fmt"
	"mime"
	"net/mail"
	"strings"
	"time"

	message "github.com/emersion/go-message"
	"github.com/emersion/go-message/charset"
)

type ParametrizedHeader struct {
	Value  string
	Params map[string]string
}

type Header = message.Header
type Address = mail.Address

type Part struct {
	Type        ParametrizedHeader
	Disposition ParametrizedHeader
	// If Body is null - this field contains expected body size.
	Size uint32
	Misc Header

	// Note, can be null, use CacheDB to request cached bodies for parts.
	Body []byte
}

// Msg struct represents a parsed E-Mail message.
//
// RFC 822-style headers are splitten into corresponding fields, remaining ones
// left in Miscfield. Multi-part MIME messages are automatically
// splitten into parts and decoded. Non-multipart bodies are represented as a
// body with single part. Headers are left empty if missing or invalid.
type Msg struct {
	Date          time.Time
	Subject       string
	From, ReplyTo Address
	To, Cc, Bcc   []Address
	Misc          Header

	Parts []Part
}

func (ph ParametrizedHeader) String() string {
	// TODO: Is it correct?
	parts := []string{ph.Value}
	for name, value := range ph.Params {
		parts = append(parts, fmt.Sprintf("%v=%v", name, value))
	}
	return strings.Join(parts, "; ")
}

func ParseParamHdr(str string) (ParametrizedHeader, error) {
	// Borrowed from https://github.com/emersion/go-message/blob/51445bfdc558d43bf3582f85b13e30210f469d35/header.go#L17
	f, params, err := mime.ParseMediaType(str)
	if err != nil {
		return ParametrizedHeader{}, err
	}
	for k, v := range params {
		params[k], _ = charset.DecodeHeader(v)
	}
	return ParametrizedHeader{f, params}, nil
}
