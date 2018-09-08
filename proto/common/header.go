package common

import (
	"bytes"

	message "github.com/emersion/go-message"
)

func ReadHeader(in []byte) (Header, error) {
	hdrsR := bytes.NewReader(in)
	entity, err := message.Read(hdrsR)
	if err != nil {
		return nil, err
	}
	return Header(entity.Header), nil
}

func WriteHeader(in Header) ([]byte, error) {
	hdrsBuf := bytes.NewBuffer([]byte{})
	hdrsWrtr, err := message.CreateWriter(hdrsBuf, message.Header(in))
	hdrsWrtr.Close() // We don't really need it, CreateWriter writes headers to underlaying Writer.
	return hdrsBuf.Bytes(), err
}
