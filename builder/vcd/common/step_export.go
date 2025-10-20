package common

import (
	"os"

	"github.com/hashicorp/packer-plugin-sdk/common"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/template/interpolate"
	"github.com/pkg/errors"
)

//go:generate packer-sdc struct-markdown
//go:generate packer-sdc mapstructure-to-hcl2 -type ExportConfig

type ExportConfig struct {
	// The name of the exported image in Open Virtualization Format (OVF).
	//
	// -> **Note:** The name of the virtual machine with the `.ovf` extension is
	// used if this option is not specified.
	Name string `mapstructure:"name"`
	// Forces the export to overwrite existing files. Defaults to `false`.
	// If set to `false`, an error is returned if the file(s) already exists.
	Force bool `mapstructure:"force"`
	// The path to the directory where the exported image will be saved.
	OutputDir OutputConfig `mapstructure:",squash"`
}

func (c *ExportConfig) Prepare(ctx *interpolate.Context, lc *LocationConfig, pc *common.PackerConfig) []error {
	var errs *packersdk.MultiError

	errs = packersdk.MultiErrorAppend(errs, c.OutputDir.Prepare(ctx, pc)...)

	// Default the name to the name of the virtual machine if not specified.
	if c.Name == "" {
		c.Name = lc.VMName
	}

	// Check if the output directory exists.
	if err := os.MkdirAll(c.OutputDir.OutputDir, c.OutputDir.DirPerm); err != nil {
		errs = packersdk.MultiErrorAppend(errs, errors.Wrap(err, "unable to make directory for export"))
	}

	// TODO(juan): Implement this
	// https://github.com/hashicorp/packer-plugin-vsphere/blob/main/builder/vsphere/common/step_export.go#L18

	return errs.Errors
}
