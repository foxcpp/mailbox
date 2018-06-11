package imap

import (
	"net/mail"
	"strings"

	eimap "github.com/emersion/go-imap"
	"github.com/foxcpp/gopher-mail/common"
)

type MessageInfo struct {
	Uid                                    uint32
	Msg                                    common.Msg
	Read, Answered, Deleted, Draft, Recent bool
	CustomFlags                            []string
}

func convertAddrList(in []*eimap.Address) []mail.Address {
	res := make([]mail.Address, len(in))
	for i, a := range in {
		res[i] = mail.Address{a.PersonalName, a.MailboxName + "@" + a.HostName}
	}
	return res
}

func MessageToInfo(msg *eimap.Message) MessageInfo {
	res := MessageInfo{}
	res.Uid = msg.Uid
	res.Msg.Date = msg.Envelope.Date
	res.Msg.Subject = msg.Envelope.Subject
	res.Msg.From = mail.Address{msg.Envelope.From[0].PersonalName, msg.Envelope.From[0].MailboxName + "@" + msg.Envelope.From[0].HostName}
	res.Msg.To = convertAddrList(msg.Envelope.To)
	res.Msg.Cc = convertAddrList(msg.Envelope.Cc)
	res.Msg.Bcc = convertAddrList(msg.Envelope.Bcc)
	res.Msg.ReplyTo = mail.Address{msg.Envelope.ReplyTo[0].PersonalName, msg.Envelope.ReplyTo[0].MailboxName + "@" + msg.Envelope.ReplyTo[0].HostName}
	for _, flag := range msg.Flags {
		switch flag {
		case eimap.SeenFlag:
			res.Read = true
		case eimap.AnsweredFlag:
			res.Answered = true
		case eimap.DeletedFlag:
			res.Deleted = true
		case eimap.DraftFlag:
			res.Draft = true
		case eimap.RecentFlag:
			res.Recent = true
		default:
			// Special IMAP flags have \ prefix.
			if !strings.HasPrefix(flag, "\\") {
				res.CustomFlags = append(res.CustomFlags, flag)
			}
		}
	}
	return res
}
