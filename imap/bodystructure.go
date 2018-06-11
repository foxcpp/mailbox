package imap

import (
	imap "github.com/emersion/go-imap"
	message "github.com/emersion/go-message"
	"github.com/foxcpp/gopher-mail/common"
)

// bodyStructToPart converts information about body part to common.Part used in
// library. Body field is left as nil.
func bodyStructToPart(s imap.BodyStructure) (res common.Part) {
	res.Type = common.ParametrizedHeader{
		s.MIMEType + "/" + s.MIMESubType,
		s.Params,
	}
	res.Misc = make(message.Header)
	if s.Extended {
		res.Disposition.Value, res.Disposition.Params = s.Disposition, s.DispositionParams
		if len(s.Language) >= 1 {
			res.Language = s.Language[0]
		}
		if len(s.Location) >= 1 {
			res.URI = s.Location[0]
		}
	}
	return res
}
