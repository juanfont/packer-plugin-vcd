package common

import (
	"context"
	"fmt"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/juanfont/packer-plugin-vcd/builder/vcd/driver"
	"github.com/vmware/go-vcloud-director/v3/govcd"
	"github.com/vmware/go-vcloud-director/v3/types/v56"
)

//go:generate packer-sdc struct-markdown
//go:generate packer-sdc mapstructure-to-hcl2 -type ExportToCatalogConfig

// ExportToCatalogConfig defines configuration for exporting the built VM as a vApp template.
// This is separate from the ISO catalog (CatalogConfig) which is used for ISO storage during build.
type ExportToCatalogConfig struct {
	// The name of the catalog to export the vApp template to.
	// This catalog must already exist in the organization.
	Catalog string `mapstructure:"catalog"`

	// The name for the vApp template in the catalog.
	// If not set, defaults to the VM name.
	TemplateName string `mapstructure:"template_name"`

	// Description for the vApp template.
	Description string `mapstructure:"description"`

	// If true, overwrite an existing template with the same name.
	// Defaults to false.
	Overwrite bool `mapstructure:"overwrite"`
}

func (c *ExportToCatalogConfig) Prepare(lc *LocationConfig) []error {
	var errs []error

	if c.Catalog == "" {
		errs = append(errs, fmt.Errorf("'catalog' is required for export_to_catalog"))
	}

	// Default template name to VM name
	if c.TemplateName == "" && lc != nil {
		c.TemplateName = lc.VMName
	}

	return errs
}

type StepExportToCatalog struct {
	Config *ExportToCatalogConfig
}

func (s *StepExportToCatalog) Run(_ context.Context, state multistep.StateBag) multistep.StepAction {
	ui := state.Get("ui").(packersdk.Ui)
	d := state.Get("driver").(driver.Driver)

	if s.Config == nil {
		// No export configured, skip
		return multistep.ActionContinue
	}

	vapp, ok := state.GetOk("vapp")
	if !ok {
		state.Put("error", fmt.Errorf("no vApp found in state"))
		return multistep.ActionHalt
	}

	ui.Sayf("Exporting vApp as template to catalog: %s", s.Config.Catalog)

	// Get the catalog
	catalog, err := d.GetCatalog(s.Config.Catalog)
	if err != nil {
		state.Put("error", fmt.Errorf("error getting catalog %s: %w", s.Config.Catalog, err))
		return multistep.ActionHalt
	}

	// Check if template already exists
	if !s.Config.Overwrite {
		existingItem, err := catalog.GetCatalogItemByName(s.Config.TemplateName, false)
		if err == nil && existingItem != nil {
			state.Put("error", fmt.Errorf("template '%s' already exists in catalog '%s'. Set overwrite=true to replace it",
				s.Config.TemplateName, s.Config.Catalog))
			return multistep.ActionHalt
		}
	}

	// Create vApp template from vApp
	vappRef := vapp.(*govcd.VApp)
	description := s.Config.Description
	if description == "" {
		description = fmt.Sprintf("Packer-built template from %s", vappRef.VApp.Name)
	}

	ui.Sayf("Creating vApp template: %s (this may take a few minutes...)", s.Config.TemplateName)
	captureParams := &types.CaptureVAppParams{
		Name:        s.Config.TemplateName,
		Description: description,
		Source: &types.Reference{
			HREF: vappRef.VApp.HREF,
			Name: vappRef.VApp.Name,
		},
	}

	vappTemplate, err := catalog.CaptureVappTemplate(captureParams)
	if err != nil {
		state.Put("error", fmt.Errorf("error capturing vApp as template: %w", err))
		return multistep.ActionHalt
	}

	ui.Sayf("vApp template '%s' created successfully in catalog '%s'", vappTemplate.VAppTemplate.Name, s.Config.Catalog)

	return multistep.ActionContinue
}

func (s *StepExportToCatalog) Cleanup(state multistep.StateBag) {
	// No cleanup needed - we want to keep the exported template
}
