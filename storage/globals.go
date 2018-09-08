package storage

import (
	"io/ioutil"
	"os"
	"path/filepath"

	yaml "gopkg.in/yaml.v2"
)

type GlobalCfg struct {
	Connection struct {
		MaxTries *int `yaml:"max_tries"`
	} `yaml:"connection"`
	Encryption struct {
		UseMasterPass *bool
		MasterKeySalt string
	}
}

var GlobalCfgDefault = GlobalCfg{}

func LoadGlobal() (*GlobalCfg, error) {
	path := filepath.Join(GetDirectory(), "global.yml")
	err := os.MkdirAll(GetDirectory(), 0700)
	if err != nil {
		return nil, err
	}

	data, err := ioutil.ReadFile(path)
	res := GlobalCfg{}
	if os.IsNotExist(err) {
		bytes, err := yaml.Marshal(res)
		if err != nil {
			panic(err)
		}

		ioutil.WriteFile(path, bytes, 0600)
	} else {
		if err != nil {
			return nil, err
		}

		res = GlobalCfg{}
		err = yaml.Unmarshal(data, &res)
		if err != nil {
			return nil, err
		}
	}

	// Set default values here.
	if res.Connection.MaxTries == nil {
		f := 5
		res.Connection.MaxTries = &f
	}
	if res.Encryption.UseMasterPass == nil {
		f := false
		res.Encryption.UseMasterPass = &f
	}

	return &res, nil
}

func SaveGlobal(cfg *GlobalCfg) error {
	path := filepath.Join(GetDirectory(), "global.yml")
	err := os.MkdirAll(GetDirectory(), 0700)
	if err != nil {
		return err
	}

	bytes, err := yaml.Marshal(*cfg)
	if err != nil {
		panic(err)
	}

	return ioutil.WriteFile(path, bytes, 0600)
}
