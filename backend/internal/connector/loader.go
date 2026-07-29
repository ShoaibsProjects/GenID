package connector

import (
	"os"

	"gopkg.in/yaml.v3"
)

func LoadManifest(path string) (*UniversalConnector, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var manifest ConnectorManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return nil, err
	}

	return &UniversalConnector{Manifest: manifest}, nil
}