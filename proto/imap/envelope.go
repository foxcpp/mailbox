package imap

import (
	"strings"

	eimap "github.com/emersion/go-imap"
	"github.com/foxcpp/mailbox/proto/common"
)

type MessageInfo struct {
	UID                      uint32
	Readen, Answered, Recent bool
	CustomTags               []string

	common.Msg
}

func convertAddrList(in []*eimap.Address) []common.Address {
	res := make([]common.Address, len(in))
	for i, a := range in {
		res[i] = common.Address{a.PersonalName, a.MailboxName + "@" + a.HostName}
	}
	return res
}

func MessageToInfo(msg *eimap.Message) MessageInfo {
	res := MessageInfo{}
	res.UID = msg.Uid
	res.Msg.Date = msg.Envelope.Date
	res.Msg.Subject = msg.Envelope.Subject
	res.Msg.From = common.Address{msg.Envelope.From[0].PersonalName, msg.Envelope.From[0].MailboxName + "@" + msg.Envelope.From[0].HostName}
	res.Msg.To = convertAddrList(msg.Envelope.To)
	res.Msg.Cc = convertAddrList(msg.Envelope.Cc)
	res.Msg.Bcc = convertAddrList(msg.Envelope.Bcc)
	res.Msg.ReplyTo = common.Address{msg.Envelope.ReplyTo[0].PersonalName, msg.Envelope.ReplyTo[0].MailboxName + "@" + msg.Envelope.ReplyTo[0].HostName}
	for _, flag := range msg.Flags {
		switch flag {
		case eimap.SeenFlag:
			res.Readen = true
		case eimap.AnsweredFlag:
			res.Answered = true
		case eimap.RecentFlag:
			res.Recent = true
		default:
			// Special IMAP flags have \ prefix.
			if !strings.HasPrefix(flag, "\\") {
				res.CustomTags = append(res.CustomTags, flag)
			}
		}
	}
	return res
}
