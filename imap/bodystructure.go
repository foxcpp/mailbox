package imap

import (
	imap "github.com/emersion/go-imap"
	message "github.com/emersion/go-message"
	"github.com/foxcpp/gopher-mail/common"
)

// bodyStructToPart converts information about body part to common.Part used in
// library. Body field is left as nil.
func bodyStructToPart(s imap.BodyStructure) (res common.Part) {
	res.Type = common.BodyType{
		s.MIMEType + "/" + s.MIMESubType,
		s.Params,
	}
	res.Misc = make(message.Header)
	// TODO: Reencode "extended" information.
	return res
}
