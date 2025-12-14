package main

// AppConfig represents the canonical (v2) configuration structure.
// The code always uses this structure, regardless of the underlying format.
type AppConfig struct {
	Database DatabaseConfig `yaml:"database" json:"database"`
	Server   ServerConfig   `yaml:"server" json:"server"`
}

type DatabaseConfig struct {
	Host     string `yaml:"host" json:"host"`
	Port     int    `yaml:"port" json:"port"`
	Name     string `yaml:"name" json:"name"`
	User     string `yaml:"user" json:"user"`
	Password string `yaml:"password" json:"password"`
}

type ServerConfig struct {
	Host string `yaml:"host" json:"host"`
	Port int    `yaml:"port" json:"port"`
}

// v2 (canonical) configuration format
const v2Config = `
database:
  host: db.example.com
  port: 5432
  name: myapp
  user: admin
  password: secret

server:
  host: 0.0.0.0
  port: 8080
`

// v1 (legacy) configuration format - different structure
// This uses "db" instead of "database", and has different nesting
const v1Config = `
# Legacy v1 configuration format
db:
  hostname: legacy-db.example.com  # was "host" in v2
  port: 3306
  database: oldapp                 # was "name" in v2
  username: root                   # was "user" in v2
  pass: legacy-secret              # was "password" in v2

http:
  listen_host: 127.0.0.1           # was "server.host" in v2
  listen_port: 3000                # was "server.port" in v2
`

// v1ToV2Mappings defines path transformations from v1 to v2 format.
var v1ToV2Mappings = []PathMapping{
	// Database mappings
	{Canonical: "/database/host", Source: "/db/hostname"},
	{Canonical: "/database/port", Source: "/db/port"},
	{Canonical: "/database/name", Source: "/db/database"},
	{Canonical: "/database/user", Source: "/db/username"},
	{Canonical: "/database/password", Source: "/db/pass"},
	// Server mappings
	{Canonical: "/server/host", Source: "/http/listen_host"},
	{Canonical: "/server/port", Source: "/http/listen_port"},
}
