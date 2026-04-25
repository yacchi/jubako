package main

import "github.com/yacchi/jubako/layer"

const (
	layerAuth        = layer.Name("auth")
	layerCredentials = layer.Name("credentials")
	pathCredential   = "/credential"
)

type AuthConfig struct {
	CredentialBackend string `json:"credential_backend"`
}

type Credential struct {
	BaseURL     string `json:"base_url"`
	AccessToken string `json:"access_token" jubako:"sensitive" storage:"keyring"`
	SecretRef   string `json:"secret_ref,omitempty"`
}

type Config struct {
	Auth       AuthConfig             `json:"auth"`
	Credential map[string]*Credential `json:"credential"`
}
