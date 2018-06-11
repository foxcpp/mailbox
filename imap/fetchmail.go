package imap

import (
	"errors"
	"strconv"

	eimap "github.com/emersion/go-imap"
	message "github.com/emersion/go-message"
	"github.com/foxcpp/gopher-mail/common"
)

// FetchMailText requests text parts of message with specified uid from specified directory.
// Returned Msg object will contain message headers, text/plain, text/html parts and information (!)
// about other parts (body slice will be nil).
func (c *Client) FetchPartialMail(dir string, uid uint32, filter func(string, string) bool) (*common.Msg, error) {
	_, err := c.cl.Select(dir, true)
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
	res := MessageToInfo(msgStruct).Msg
	if msgStruct.BodyStructure.MIMEType == "multipart" {
		// Request only parts accepted by filter.
		for i, partStruct := range msgStruct.BodyStructure.Parts {
			if filter(partStruct.MIMEType, partStruct.MIMESubType) {
				part, err := c.DownloadPart(uid, i)
				if err != nil {
					return nil, err
				}
				res.Parts = append(res.Parts, *part)
			} else {
				res.Parts = append(res.Parts, bodyStructToPart(*partStruct))
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
			part.Type = common.BodyType{
				msgStruct.BodyStructure.MIMEType + "/" + msgStruct.BodyStructure.MIMESubType,
				msgStruct.BodyStructure.Params,
			}

			part.Body = make([]byte, v.Len())
			_, err := v.Read(part.Body)
			if err != nil {
				return nil, err
			}
			res.Parts = append(res.Parts, part)
		}
	}
	return &res, nil
}

func (c *Client) DownloadPart(uid uint32, partIndex int) (*common.Part, error) {
	hdr, err := c.downloadPartHeader(uid, partIndex)
	if err != nil {
		return nil, err
	}
	body, err := c.downloadPartBody(uid, partIndex)
	if err != nil {
		return nil, err
	}

	res := common.Part{}
	res.Type.T, res.Type.Params, err = hdr.ContentType()
	if err != nil {
		return nil, err
	}
	res.Body = body
	return &res, nil
}

func (c *Client) downloadPartHeader(uid uint32, partIndex int) (message.Header, error) {
	out := make(chan *eimap.Message, 1)
	seqset := eimap.SeqSet{}
	seqset.AddNum(uid)
	err := c.cl.UidFetch(&seqset, []eimap.FetchItem{eimap.FetchItem("BODY.PEEK[" + strconv.Itoa(partIndex+1) + ".MIME]")}, out)
	if err != nil {
		return nil, err
	}
	msgHdr := <-out
	for _, v := range msgHdr.Body {
		m, err := message.Read(v)
		if err != nil {
			return nil, err
		}
		return m.Header, nil
	}
	return nil, errors.New("DownloadPart: no data returned by server")
}

func (c *Client) downloadPartBody(uid uint32, partIndex int) ([]byte, error) {
	out := make(chan *eimap.Message, 1)
	seqset := eimap.SeqSet{}
	seqset.AddNum(uid)
	err := c.cl.UidFetch(&seqset, []eimap.FetchItem{eimap.FetchItem("BODY.PEEK[" + strconv.Itoa(partIndex+1) + "]")}, out)
	if err != nil {
		return nil, err
	}
	msgBody := <-out
	for _, v := range msgBody.Body {
		buf := make([]byte, v.Len())
		_, err := v.Read(buf)
		return buf, err
	}
	return nil, errors.New("DownloadPart: no data returned by server")
}
