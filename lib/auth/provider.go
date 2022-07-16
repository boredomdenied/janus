package auth

import (
	"log"

	"github.com/markbates/goth/providers/openidConnect"
)

// OpenIDConfig holds the Single Sign On parameters.
type OpenIDConfig struct {
	Key    string
	Secret string
	Scopes []string

	CallbackURL  string
	DiscoveryURL string
}

// Provider creates a new OpenID provider.
func Provider(cfg *OpenIDConfig) (*openidConnect.Provider, error) {
	log.Println("Config: ", cfg)
	return openidConnect.New(cfg.Key, cfg.Secret, cfg.CallbackURL, cfg.DiscoveryURL, cfg.Scopes...)
}
