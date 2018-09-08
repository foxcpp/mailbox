package imap

import (
	imap "github.com/emersion/go-imap"
	"github.com/foxcpp/mailbox/proto/common"
)

// bodyStructToPart converts information about body part to common.Part used in
// library. Body field is left as nil.
func bodyStructToPart(s imap.BodyStructure) (res common.Part) {
	res.Type = common.ParametrizedHeader{
		s.MIMEType + "/" + s.MIMESubType,
		s.Params,
	}
	res.Misc = make(common.Header)
	res.Size = s.Size
	if s.Extended {
		res.Disposition.Value, res.Disposition.Params = s.Disposition, s.DispositionParams
		if len(s.Language) >= 1 {
			res.Misc.Add("Content-Language", s.Language[0])
		}
		if len(s.Location) >= 1 {
			res.Misc.Add("Content-Location", s.Location[0])
		}
	}
	return res
}
