package config

import (
	"encoding/json"
	"gopkg.in/yaml.v3"
	"io/ioutil"
)

type LoaderType string

const (
	LoaderTypeYml  = "yml"
	LoaderTypeJson = "json"
)

type Loader interface {
	Load(result interface{}) error
}

func NewLoader(loaderType LoaderType, endpoint string) Loader {
	switch loaderType {
	case LoaderTypeYml:
		return YmlLoader{
			path: endpoint,
		}
	case LoaderTypeJson:
		return JsonLoader{
			path: endpoint,
		}
	}
	return nil
}

type YmlLoader struct {
	path string
}

func (l YmlLoader) Load(result interface{}) error {
	data, err := ioutil.ReadFile(l.path)
	if err != nil {
		return err
	}

	if err = yaml.Unmarshal(data, result); err != nil {
		return err
	}

	return nil
}

type JsonLoader struct {
	path string
}

func (l JsonLoader) Load(result interface{}) error {
	data, err := ioutil.ReadFile(l.path)
	if err != nil {
		return err
	}

	if err = json.Unmarshal(data, result); err != nil {
		return err
	}

	return nil
}
