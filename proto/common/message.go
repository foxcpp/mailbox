package common

import (
	"fmt"
	"net/mail"
	"strings"
	"time"

	message "github.com/emersion/go-message"
)

type ParametrizedHeader struct {
	Value  string
	Params map[string]string
}

type Part struct {
	Type        ParametrizedHeader
	Disposition ParametrizedHeader
	Size        uint32
	Misc        message.Header

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
	From, ReplyTo mail.Address
	To, Cc, Bcc   []mail.Address
	Misc          message.Header

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
