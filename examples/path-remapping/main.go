// Example: Path Remapping with jubako struct tags
//
// This example demonstrates how to use jubako tags to remap nested YAML/TOML/JSON
// paths to flat struct fields. This is useful when:
// - Config files are nested for human readability
// - Application code prefers flat structs for convenience
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/yacchi/jubako"
	"github.com/yacchi/jubako/layer/mapdata"
)

// ServerConfig demonstrates path remapping using jubako struct tags.
// The jubako tag specifies the source path in the config file.
//
// YAML structure (nested):
//
//	server:
//	  host: "0.0.0.0"
//	  port: 8080
//	  http:
//	    read_timeout: 10
//	    write_timeout: 30
//	  cookie:
//	    secret: "xxx"
//
// Go struct (flat):
type ServerConfig struct {
	Host             string `json:"host" jubako:"/server/host"`
	Port             int    `json:"port" jubako:"/server/port"`
	HTTPReadTimeout  int    `json:"http_read_timeout" jubako:"/server/http/read_timeout"`
	HTTPWriteTimeout int    `json:"http_write_timeout" jubako:"/server/http/write_timeout"`
	CookieSecret     string `json:"cookie_secret" jubako:"/server/cookie/secret"`
	// Fields with jubako:"-" are skipped during remapping
	InternalField string `json:"internal" jubako:"-"`
}

func main() {
	// Simulate nested config data (as would be parsed from YAML/TOML/JSON)
	configData := map[string]any{
		"server": map[string]any{
			"host": "0.0.0.0",
			"port": 8080,
			"http": map[string]any{
				"read_timeout":  10,
				"write_timeout": 30,
			},
			"cookie": map[string]any{
				"secret": "super-secret-key",
			},
		},
		// This would be mapped to InternalField but is skipped due to jubako:"-"
		"internal": "should be ignored",
	}

	// Create store
	store := jubako.New[ServerConfig]()

	// Print mapping table for verification
	fmt.Println("=== Path Mapping Table ===")
	table := store.MappingTable()
	if table != nil {
		fmt.Println(table)
		// Or access programmatically:
		// for _, m := range table.Mappings {
		//     fmt.Printf("%s <- %s (skipped: %v)\n", m.FieldKey, m.SourcePath, m.Skipped)
		// }
	}

	// Add layer
	err := store.Add(mapdata.New("config", configData))
	if err != nil {
		log.Fatal(err)
	}

	// Load configuration
	err = store.Load(context.Background())
	if err != nil {
		log.Fatal(err)
	}

	// Get the resolved config
	config := store.Get()

	// Print results
	fmt.Println("=== Resolved Configuration ===")
	fmt.Printf("Host:             %s\n", config.Host)
	fmt.Printf("Port:             %d\n", config.Port)
	fmt.Printf("HTTPReadTimeout:  %d\n", config.HTTPReadTimeout)
	fmt.Printf("HTTPWriteTimeout: %d\n", config.HTTPWriteTimeout)
	fmt.Printf("CookieSecret:     %s\n", config.CookieSecret)
	fmt.Printf("InternalField:    %q (should be empty)\n", config.InternalField)
}
