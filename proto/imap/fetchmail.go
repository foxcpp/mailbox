package imap

import (
	"errors"
	"strconv"

	eimap "github.com/emersion/go-imap"
	message "github.com/emersion/go-message"
	"github.com/foxcpp/mailbox/proto/common"
)

// FetchPartialMail requests text parts of message with specified uid from specified directory.
// Returned Msg object will contain message headers, text/plain, text/html parts and information (!)
// about other parts (body slice will be nil).
func (c *Client) FetchPartialMail(dir string, uid uint32, filter func(string, string) bool) (*MessageInfo, error) {
	c.stopIdle()
	defer c.resumeIdle()
	c.IOLock.Lock()
	defer c.IOLock.Unlock()

	_, err := c.cl.Select(dir, true)
	if err != nil {
		return nil, err
	}
	defer c.cl.Close()

	seqset := eimap.SeqSet{}
	seqset.AddNum(uid)

	out := make(chan *eimap.Message, 1)
	err = c.cl.UidFetch(&seqset, []eimap.FetchItem{eimap.FetchEnvelope, eimap.FetchBodyStructure}, out)
	if err != nil {
		return nil, err
	}
	msgStruct := <-out
	if msgStruct == nil {
		return nil, errors.New("fetchmail: invalid uid")
	}
	res := MessageToInfo(msgStruct)
	if msgStruct.BodyStructure.MIMEType == "multipart" {
		// Request only parts accepted by filter.
		for i, partStruct := range msgStruct.BodyStructure.Parts {
			if filter(partStruct.MIMEType, partStruct.MIMESubType) {
				// TODO: Can we download all parts at once?
				part, err := c.downloadPart(uid, i)
				if err != nil {
					return nil, err
				}
				res.Msg.Parts = append(res.Msg.Parts, *part)
			} else {
				res.Msg.Parts = append(res.Msg.Parts, bodyStructToPart(*partStruct))
			}
		}
	} else if filter(msgStruct.BodyStructure.MIMEType, msgStruct.BodyStructure.MIMESubType) {
		// Request entire message.
		out := make(chan *eimap.Message, 1)
		err := c.cl.UidFetch(&seqset, []eimap.FetchItem{"BODY.PEEK[TEXT]"}, out)
		if err != nil {
			return nil, err
		}
		msgBody := <-out
		for _, v := range msgBody.Body {
			part := bodyStructToPart(*msgStruct.BodyStructure)
			part.Type = common.ParametrizedHeader{
				msgStruct.BodyStructure.MIMEType + "/" + msgStruct.BodyStructure.MIMESubType,
				msgStruct.BodyStructure.Params,
			}

			part.Body = make([]byte, v.Len())
			_, err := v.Read(part.Body)
			if err != nil {
				return nil, err
			}
			res.Msg.Parts = append(res.Msg.Parts, part)
		}
	}
	return &res, nil
}

func (c *Client) DownloadPart(dir string, uid uint32, partIndex int) (*common.Part, error) {
	c.stopIdle()
	defer c.resumeIdle()
	c.IOLock.Lock()
	defer c.IOLock.Unlock()

	_, err := c.cl.Select(dir, true)
	if err != nil {
		return nil, err
	}

	return c.downloadPart(uid, partIndex)
}

func (c *Client) downloadPart(uid uint32, partIndex int) (*common.Part, error) {
	// TODO: Allow to fetch multiple parts in one operation.
	headerFI := eimap.FetchItem("BODY.PEEK[" + strconv.Itoa(partIndex+1) + ".MIME]")
	bodyFI := eimap.FetchItem("BODY.PEEK[" + strconv.Itoa(partIndex+1) + "]")

	// .PEEK specifier will be omitted in response.
	headerFIRes := eimap.FetchItem("BODY[" + strconv.Itoa(partIndex+1) + ".MIME]")
	bodyFIRes := eimap.FetchItem("BODY[" + strconv.Itoa(partIndex+1) + "]")

	seqset := eimap.SeqSet{}
	seqset.AddNum(uid)

	out := make(chan *eimap.Message, 1)
	err := c.cl.UidFetch(&seqset, []eimap.FetchItem{headerFI, bodyFI}, out)
	if err != nil {
		return nil, err
	}
	msg := <-out

	hdr := (*message.Entity)(nil)
	buf := ([]byte)(nil)

	for name, v := range msg.Body {
		if name.FetchItem() == headerFIRes {
			// Parse MIME header.
			hdr, err = message.Read(v)
			if err != nil {
				return nil, err
			}
		} else if name.FetchItem() == bodyFIRes {
			// Parse message body.
			buf = make([]byte, v.Len())
			v.Read(buf)
		}
	}

	res := common.Part{}

	// Split MIME header.
	res.Type.Value, res.Type.Params, _ = hdr.Header.ContentType()
	hdr.Header.Del("Content-Type")
	res.Disposition.Value, res.Disposition.Params, _ = hdr.Header.ContentDisposition()
	hdr.Header.Del("Content-Disposition")
	res.Language = hdr.Header.Get("Content-Language")
	hdr.Header.Del("Content-Language")
	res.URI = hdr.Header.Get("Content-Location")
	res.Misc = hdr.Header

	res.Body = buf

	return &res, nil
}
