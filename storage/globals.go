package storage

import (
	"io/ioutil"
	"os"
	"path/filepath"

	yaml "gopkg.in/yaml.v2"
)

type GlobalCfg struct {
	Connection struct {
		MaxTries uint `yaml:"max_tries"`
	} `yaml:"connection"`
}

var GlobalCfgDefault = GlobalCfg{}

func LoadGlobal() (*GlobalCfg, error) {
	path := filepath.Join(GetDirectory(), "global.yml")
	err := os.MkdirAll(GetDirectory(), os.ModePerm)
	if err != nil {
		return nil, err
	}

	data, err := ioutil.ReadFile(path)
	if os.IsNotExist(err) {
		bytes, err := yaml.Marshal(GlobalCfgDefault)
		if err != nil {
			panic(err)
		}

		ioutil.WriteFile(path, bytes, os.ModePerm)
		return &GlobalCfgDefault, nil
	}
	if err != nil {
		return nil, err
	}

	res := GlobalCfg{}
	err = yaml.Unmarshal(data, &res)
	return &res, err
}
