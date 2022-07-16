package janus

import (
	"fmt"
	"io/ioutil"
	"log"

	"github.com/pelletier/go-toml/v2"
	"gitlab.operationuplift.work/operations/development/janus/lib/provisioner"
	"gitlab.operationuplift.work/operations/development/janus/lib/useradmin"
)

// Config holds together the full janus config.
type Config struct {
	Provisioner *provisioner.Config
	UserAdmin   *useradmin.Config
}

// MustLoadConfig loads a TOML-formatted configuration from the given file.
// It terminates the process on error.
func MustLoadConfig(file string) *Config {
	res, err := LoadConfig(file)
	if err != nil {
		log.Fatalf("Loading configuration file %q: %v", file, err)
	}
	return res
}

// LoadConfig loads a TOML-formatted configuration from the given file.
func LoadConfig(file string) (*Config, error) {
	data, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("reading file: %w", err)
	}

	res := &Config{}
	if err := toml.Unmarshal(data, &res); err != nil {
		return nil, fmt.Errorf("decoding TOML: %w", err)
	}

	return res, nil
}
