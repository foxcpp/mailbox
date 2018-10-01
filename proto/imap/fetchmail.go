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

	_, err := c.ensureSelected(dir, true)
	if err != nil {
		return nil, err
	}

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
		res.Msg.Parts = make([]common.Part, len(msgStruct.BodyStructure.Parts))
		// Request only parts accepted by filter.
		toDownload := make([]int, 0, len(msgStruct.BodyStructure.Parts))
		for i, partStruct := range msgStruct.BodyStructure.Parts {
			if filter(partStruct.MIMEType, partStruct.MIMESubType) {
				toDownload = append(toDownload, i)
			} else {
				res.Msg.Parts[i] = bodyStructToPart(*partStruct)
			}
		}

		parts, err := c.downloadParts(uid, toDownload...)
		if err != nil {
			return nil, err
		}
		// resindx - Index in res.Parts.
		// i - index in toDownload and parts.
		for i, resindx := range toDownload {
			res.Msg.Parts[resindx] = parts[i]
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

	_, err := c.ensureSelected(dir, true)
	if err != nil {
		return nil, err
	}

	return c.downloadPart(uid, partIndex)
}

func (c *Client) downloadParts(uid uint32, partIndxs ...int) ([]common.Part, error) {
	requestFIs := make([]eimap.FetchItem, 0, len(partIndxs) * 2)
	for _, part := range partIndxs {
		requestFIs = append(requestFIs, eimap.FetchItem("BODY.PEEK[" + strconv.Itoa(part+1) + ".MIME]"))
		requestFIs = append(requestFIs, eimap.FetchItem("BODY.PEEK[" + strconv.Itoa(part+1) + "]"))
	}

	seqset := eimap.SeqSet{}
	seqset.AddNum(uid)
	out := make(chan *eimap.Message, 1)
	if err := c.cl.UidFetch(&seqset, requestFIs, out); err != nil {
		return nil, err
	}
	msg := <- out

	res := make([]common.Part, len(partIndxs))
	for i, part := range partIndxs {
		for fi, bodyLiteral := range msg.Body {
			if fi.FetchItem() == eimap.FetchItem("BODY[" + strconv.Itoa(part+1) + ".MIME]") {
				hdr, err := message.Read(bodyLiteral)
				if err != nil {
					return nil, err
				}

				res[i].Type.Value, res[i].Type.Params, _ = hdr.Header.ContentType()
				hdr.Header.Del("Content-Type")
				res[i].Disposition.Value, res[i].Disposition.Params, _ = hdr.Header.ContentDisposition()
				hdr.Header.Del("Content-Disposition")
				res[i].Misc = common.Header(hdr.Header)
			}
			if fi.FetchItem() == eimap.FetchItem("BODY[" + strconv.Itoa(part+1) + "]") {
				buf := make([]byte, bodyLiteral.Len())
				if _, err := bodyLiteral.Read(buf); err != nil {
					return nil, err
				}
				res[i].Body = buf
				res[i].Size = uint32(len(buf))
			}
		}
	}

	return res, nil
}

func (c *Client) downloadPart(uid uint32, partIndex int) (*common.Part, error) {
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
	res.Misc = common.Header(hdr.Header)

	res.Body = buf
	res.Size = uint32(len(buf))

	return &res, nil
}
