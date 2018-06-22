package storage

import (
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/foxcpp/mailbox/proto/imap"
	"github.com/ugorji/go/codec"
)

func CachedDirs(accountId string) []string {
	res := []string{}
	info, err := ioutil.ReadDir(filepath.Join(GetDirectory(), "cache", accountId))
	if err != nil {
		return []string{}
	}
	for _, dir := range info {
		if dir.IsDir() || filepath.Ext(dir.Name()) != ".messages" {
			continue
		}
		res = append(res, basename(dir.Name()))
	}
	return res
}

type CacheFormat struct {
	UidValidity uint32
	Msgs        []imap.MessageInfo
}

func ReadCache(accountId, dir string) (uidvalidity uint32, msgs []imap.MessageInfo, err error) {
	path := filepath.Join(GetDirectory(), "cache", accountId, dir+".messages")
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return 0, nil, err
	}

	res := CacheFormat{}

	enc := codec.NewDecoderBytes(data, &codec.CborHandle{})
	err = enc.Decode(&res)
	return res.UidValidity, res.Msgs, err
}

func WriteCache(accountId, dir string, uidValidity uint32, msgs []imap.MessageInfo) error {
	path := filepath.Join(GetDirectory(), "cache", accountId, dir+".messages")
	err := os.MkdirAll(filepath.Join(GetDirectory(), "cache", accountId), 0700)
	if err != nil {
		return err
	}

	cache := CacheFormat{uidValidity, msgs}

	buf := []byte{}

	enc := codec.NewEncoderBytes(&buf, &codec.CborHandle{})
	err = enc.Encode(cache)
	if err != nil {
		return err
	}

	return ioutil.WriteFile(path, buf, 0600)
}
