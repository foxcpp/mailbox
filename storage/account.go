package storage

import (
	"errors"
	"io/ioutil"
	"path/filepath"
	"strings"

	yaml "gopkg.in/yaml.v2"
)

type AccountCfg struct {
	Name   string
	Server struct {
		Imap struct {
			Host       string
			Port       uint
			Encryption string
		}
		Smtp struct {
			Host       string
			Port       uint
			Encryption string
		}
	}
	Credentials struct {
		User string
		Pass string // IV + encrypted password actually
	}
	// TODO: Overrides?
}

func LoadAccount(name string) (*AccountCfg, error) {
	path := filepath.Join(GetDirectory(), "accounts", name+".yml")

	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	res := AccountCfg{}
	err = yaml.Unmarshal(data, &res)
	if err != nil {
		return nil, err
	}
	if res.Server.Imap.Encryption != "tls" &&
		res.Server.Imap.Encryption != "starttls" ||
		res.Server.Smtp.Encryption != "tls" &&
			res.Server.Smtp.Encryption != "starttls" {
		return nil, errors.New("LoadAccount: encryption field may contain only 'tls' or 'starttls' strings")
	}
	return &res, nil
}

func basename(s string) string {
	n := strings.LastIndexByte(s, '.')
	if n >= 0 {
		return s[:n]
	}
	return s
}

func LoadAllAccounts() (map[string]AccountCfg, error) {
	res := make(map[string]AccountCfg)
	info, err := ioutil.ReadDir(filepath.Join(GetDirectory(), "accounts"))
	if err != nil {
		return nil, err
	}
	for _, i := range info {
		if i.IsDir() || filepath.Ext(i.Name()) != ".yml" {
			continue
		}

		cfg, err := LoadAccount(basename(i.Name()))
		if err != nil {
			return nil, err
		}
		res[basename(i.Name())] = *cfg
	}
	return res, nil
}
