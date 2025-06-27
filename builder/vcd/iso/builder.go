// Copyright 2025 Juan Font
// BSD-3-Clause

package iso

import "github.com/hashicorp/packer-plugin-sdk/multistep"

type Builder struct {
	config Config
	runner multistep.Runner
}
