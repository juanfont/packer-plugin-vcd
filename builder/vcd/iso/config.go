// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:generate packer-sdc struct-markdown
//go:generate packer-sdc mapstructure-to-hcl2 -type Config

package iso

import (
	"github.com/juanfont/packer-plugin-vcd/builder/vcd/common"

	packerCommon "github.com/hashicorp/packer-plugin-sdk/common"
	"github.com/hashicorp/packer-plugin-sdk/communicator"
	"github.com/hashicorp/packer-plugin-sdk/multistep/commonsteps"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/template/config"
	"github.com/hashicorp/packer-plugin-sdk/template/interpolate"
)

type Config struct {
	packerCommon.PackerConfig `mapstructure:",squash"`
	commonsteps.HTTPConfig    `mapstructure:",squash"`
	commonsteps.CDConfig      `mapstructure:",squash"`

	common.ConnectConfig      `mapstructure:",squash"`
	common.CatalogConfig      `mapstructure:",squash"`
	CreateConfig              `mapstructure:",squash"`
	common.LocationConfig     `mapstructure:",squash"`
	common.HardwareConfig     `mapstructure:",squash"`
	commonsteps.ISOConfig     `mapstructure:",squash"`
	common.BootCommandConfig  `mapstructure:",squash"`
	// common.CDRomConfig                `mapstructure:",squash"` // we will probably need this
	common.RemoveNetworkAdapterConfig `mapstructure:",squash"`
	common.RunConfig                  `mapstructure:",squash"`
	Comm                              communicator.Config `mapstructure:",squash"`

	common.ShutdownConfig `mapstructure:",squash"`

	// The configuration for exporting the virtual machine to an OVF.
	// The virtual machine is not exported if [export configuration](#export-configuration) is not specified.
	Export *common.ExportConfig `mapstructure:"export"`

	// Export the virtual machine to a catalog.
	// The virtual machine will not be exported if no [export to catalog configuration](#export-to-catalog-configuration) is specified.
	ExportToCatalog *common.ExportToCatalogConfig `mapstructure:"export_to_catalog"`

	ctx interpolate.Context
}

// Prepare processes and validates the configuration for building and exporting.
// It returns a list of warnings and an error if validation fails.
func (c *Config) Prepare(raws ...interface{}) ([]string, error) {
	err := config.Decode(c, &config.DecodeOpts{
		PluginType:         common.BuilderId,
		Interpolate:        true,
		InterpolateContext: &c.ctx,
		InterpolateFilter: &interpolate.RenderFilter{
			Exclude: []string{
				"boot_command",
			},
		},
	}, raws...)
	if err != nil {
		return nil, err
	}

	warnings := make([]string, 0)
	errs := new(packersdk.MultiError)

	if c.ISOUrls != nil || c.RawSingleISOUrl != "" {
		isoWarnings, isoErrs := c.ISOConfig.Prepare(&c.ctx)
		warnings = append(warnings, isoWarnings...)
		errs = packersdk.MultiErrorAppend(errs, isoErrs...)
	}

	errs = packersdk.MultiErrorAppend(errs, c.ConnectConfig.Prepare()...)
	errs = packersdk.MultiErrorAppend(errs, c.CatalogConfig.Prepare()...)
	errs = packersdk.MultiErrorAppend(errs, c.CreateConfig.Prepare()...)
	errs = packersdk.MultiErrorAppend(errs, c.LocationConfig.Prepare()...)
	errs = packersdk.MultiErrorAppend(errs, c.HardwareConfig.Prepare()...)
	errs = packersdk.MultiErrorAppend(errs, c.BootCommandConfig.Prepare(&c.ctx)...)
	errs = packersdk.MultiErrorAppend(errs, c.HTTPConfig.Prepare(&c.ctx)...)
	errs = packersdk.MultiErrorAppend(errs, c.CDConfig.Prepare(&c.ctx)...)
	errs = packersdk.MultiErrorAppend(errs, c.Comm.Prepare(&c.ctx)...)

	shutdownWarnings, shutdownErrs := c.ShutdownConfig.Prepare(c.Comm)
	warnings = append(warnings, shutdownWarnings...)
	errs = packersdk.MultiErrorAppend(errs, shutdownErrs...)

	if c.Export != nil {
		errs = packersdk.MultiErrorAppend(errs, c.Export.Prepare(&c.ctx, &c.LocationConfig, &c.PackerConfig)...)
	}
	if c.ExportToCatalog != nil {
		errs = packersdk.MultiErrorAppend(errs, c.ExportToCatalog.Prepare(&c.LocationConfig)...)
	}

	if len(errs.Errors) > 0 {
		return warnings, errs
	}

	return warnings, nil
}
