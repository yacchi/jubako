package main

import "github.com/yacchi/jubako/layer"

const (
	layerAuth        = layer.Name("auth")
	layerCredentials = layer.Name("credentials")
	pathCredential   = "/credential"
	keyringService   = "github.com/yacchi/jubako/examples/coordinated-keyring"
)

type AuthConfig struct {
	CredentialBackend string `json:"credential_backend"`
}

type Profile struct {
	SpaceID string `json:"space_id"`
}

type Credential struct {
	BaseURL      string `json:"base_url"`
	UserID       string `json:"user_id"`
	AccessToken  string `json:"access_token" jubako:"sensitive" storage:"keyring"`
	RefreshToken string `json:"refresh_token" jubako:"sensitive" storage:"keyring"`
	SecretRef    string `json:"secret_ref,omitempty"`
}

type Config struct {
	Auth       AuthConfig             `json:"auth"`
	Profile    map[string]*Profile    `json:"profile"`
	Credential map[string]*Credential `json:"credential"`
}
