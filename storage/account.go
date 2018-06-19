package storage

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	yaml "gopkg.in/yaml.v2"
)

type AccountCfg struct {
	AccountName string
	SenderName  string
	SenderEmail string
	Server      struct {
		Imap struct {
			Host       string
			Port       uint16
			Encryption string
		}
		Smtp struct {
			Host       string
			Port       uint16
			Encryption string
		}
	}
	Credentials struct {
		User string
		Pass string // IV + encrypted password actually
	}
	Dirs struct {
		Drafts string
		Sent   string
		Trash  string
	}
	CopyToSent *bool
}

func LoadAccount(name string) (*AccountCfg, error) {
	path := filepath.Join(GetDirectory(), "accounts", name+".yml")
	err := os.MkdirAll(filepath.Join(GetDirectory(), "accounts"), os.ModePerm)
	if err != nil {
		return nil, fmt.Errorf("loadaccount %v: %v", name, err)
	}

	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("loadaccount %v: %v", name, err)
	}

	res := AccountCfg{}
	err = yaml.Unmarshal(data, &res)
	if err != nil {
		return nil, fmt.Errorf("loadaccount %v: %v", name, err)
	}
	if res.Server.Imap.Encryption != "tls" &&
		res.Server.Imap.Encryption != "starttls" ||
		res.Server.Smtp.Encryption != "tls" &&
			res.Server.Smtp.Encryption != "starttls" {
		return nil, fmt.Errorf("loadaccount %v: encryption field may contain only 'tls' or 'starttls' strings", name)
	}

	// Assign default values to possibly-missing fields.
	if res.Dirs.Drafts == "" {
		res.Dirs.Drafts = "Drafts"
	}
	if res.Dirs.Sent == "" {
		res.Dirs.Sent = "Sent"
	}
	if res.Dirs.Trash == "" {
		res.Dirs.Trash = "Trash"
	}
	if res.CopyToSent == nil {
		copyToSent := true
		res.CopyToSent = &copyToSent
	}

	// Write new default values to file if they are missing.
	SaveAccount(name, res)

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
	err := os.MkdirAll(filepath.Join(GetDirectory(), "accounts"), os.ModePerm)
	if err != nil {
		return nil, fmt.Errorf("loadallaccounts: %v", err)
	}
	info, err := ioutil.ReadDir(filepath.Join(GetDirectory(), "accounts"))
	if err != nil {
		return nil, fmt.Errorf("loadallaccounts: %v", err)
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

func SaveAccount(name string, conf AccountCfg) error {
	path := filepath.Join(GetDirectory(), "accounts", name+".yml")

	err := os.MkdirAll(filepath.Join(GetDirectory(), "accounts"), os.ModePerm)
	if err != nil {
		return fmt.Errorf("saveaccount %v: %v", name, err)
	}

	bytes, err := yaml.Marshal(conf)
	if err != nil {
		return fmt.Errorf("saveaccount %v: %v", name, err)
	}

	return ioutil.WriteFile(path, bytes, os.ModePerm)
}

func DeleteAccount(name string) error {
	path := filepath.Join(GetDirectory(), "accounts", name+".yml")

	err := os.MkdirAll(filepath.Join(GetDirectory(), "accounts"), os.ModePerm)
	if err != nil {
		return fmt.Errorf("deleteaccount %v: %v", name, err)
	}

	return os.Remove(path)
}
