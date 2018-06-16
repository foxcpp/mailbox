package storage

import (
	"io/ioutil"
	"path/filepath"

	yaml "gopkg.in/yaml.v2"
)

type GlobalCfg struct {
	Connection struct {
		MaxTries uint `yaml:"max_tries"`
	} `yaml:"connection"`
}

func LoadGlobal() (*GlobalCfg, error) {
	path := filepath.Join(GetDirectory(), "global.yml")

	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	res := GlobalCfg{}
	err = yaml.Unmarshal(data, &res)
	return &res, err
}
