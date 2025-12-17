package testdata

import "time"

// ServerConfig holds server settings.
type ServerConfig struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

// PluginConfig holds plugin settings.
type PluginConfig struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

// HostConfig holds host settings.
type HostConfig struct {
	URL  string `json:"url"`
	Port int    `json:"port"`
}

// AppConfig is a sample configuration struct for testing.
type AppConfig struct {
	Server   ServerConfig            `json:"server"`
	Database string                  `json:"database"`
	Hosts    map[string]HostConfig   `json:"hosts"`
	Plugins  []PluginConfig          `json:"plugins"`
}

// ConfigWithJubakoTags tests jubako tag handling.
type ConfigWithJubakoTags struct {
	// Absolute path
	GlobalSetting string `json:"global" jubako:"/settings/global"`
	// Relative path
	Server struct {
		Host string `json:"host" jubako:"hostname"`
	} `json:"server"`
	// Normal field
	Database string `json:"database"`
}

// NestedDynamicConfig tests multiple dynamic levels.
type NestedDynamicConfig struct {
	Servers map[string]struct {
		Ports []struct {
			Address string `json:"address"`
			Enabled bool   `json:"enabled"`
		} `json:"ports"`
	} `json:"servers"`
}

// ConfigWithTimeFields tests external package types like time.Time.
type ConfigWithTimeFields struct {
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Timeout   time.Duration `json:"timeout"`
}
