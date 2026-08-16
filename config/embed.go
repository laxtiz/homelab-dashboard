// Package config embeds the sample configuration template shipped with the
// binary. It is copied to the working directory when no config file is found.
package config

import _ "embed"

//go:embed dashboard.yaml
var Sample []byte