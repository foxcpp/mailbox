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
	Name   string
	Server struct {
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
	// TODO: Overrides?
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
