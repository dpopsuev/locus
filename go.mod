module github.com/dpopsuev/locus

go 1.25.8

require (
	github.com/BurntSushi/toml v1.6.0
	github.com/modelcontextprotocol/go-sdk v1.4.0
	github.com/smacker/go-tree-sitter v0.0.0-20240827094217-dd81d9e9be82
	github.com/spf13/cobra v1.10.2
	golang.org/x/mod v0.33.0
	golang.org/x/tools v0.42.0
)

require github.com/dpopsuev/battery v0.0.0-00010101000000-000000000000 // indirect

require (
	github.com/google/jsonschema-go v0.4.2 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/segmentio/asm v1.1.3 // indirect
	github.com/segmentio/encoding v0.5.3 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
	github.com/yosida95/uritemplate/v3 v3.0.2 // indirect
	golang.org/x/oauth2 v0.34.0 // indirect
	golang.org/x/sync v0.19.0
	golang.org/x/sys v0.41.0 // indirect
	gonum.org/v1/gonum v0.17.0
	gopkg.in/yaml.v3 v3.0.1
)

replace github.com/dpopsuev/battery => ../battery
