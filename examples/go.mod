module github.com/yacchi/jubako/examples

go 1.24

toolchain go1.24.10

require (
	github.com/yacchi/jubako v0.0.0
	github.com/yacchi/jubako/format/yaml v0.0.0
)

require gopkg.in/yaml.v3 v3.0.1 // indirect

replace (
	github.com/yacchi/jubako => ..
	github.com/yacchi/jubako/format/yaml => ../format/yaml
)
