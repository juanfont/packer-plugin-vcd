package common

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/juanfont/packer-plugin-vcd/builder/vcd/driver"
	"github.com/vmware/go-vcloud-director/v3/govcd"
	"github.com/vmware/go-vcloud-director/v3/types/v56"
)

const (
	templateStatusTimeout   = 30 * time.Minute
	templateDeleteTimeout   = 10 * time.Minute
	templateStatusPollDelay = 30 * time.Second
)

//go:generate packer-sdc struct-markdown
//go:generate packer-sdc mapstructure-to-hcl2 -type ExportToCatalogConfig

// ExportToCatalogConfig defines configuration for exporting the built VM as a vApp template.
// This is separate from the ISO catalog (CatalogConfig) which is used for ISO storage during build.
type ExportToCatalogConfig struct {
	// The name of the catalog to export the vApp template to.
	Catalog string `mapstructure:"catalog"`

	// The name for the vApp template in the catalog.
	// If not set, defaults to the VM name.
	TemplateName string `mapstructure:"template_name"`

	// Description for the vApp template.
	Description string `mapstructure:"description"`

	// If true, overwrite an existing template with the same name.
	// Defaults to false.
	Overwrite bool `mapstructure:"overwrite"`

	// If true, create the catalog if it doesn't exist.
	// Defaults to false.
	CreateCatalog bool `mapstructure:"create_catalog"`
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

	// Eject ISO before capturing - VCD cannot capture vApp with mounted media
	if isoMounted, ok := state.GetOk("iso_mounted"); ok && isoMounted.(bool) {
		vm := state.Get("vm").(driver.VirtualMachine)
		catalogName := state.Get("catalog_name").(string)
		mediaName := state.Get("uploaded_media_name").(string)

		ui.Sayf("Ejecting ISO before export: %s", mediaName)
		if err := vm.EjectMedia(catalogName, mediaName); err != nil {
			ui.Errorf("Warning: failed to eject ISO: %s", err)
			// Continue anyway - the capture might still work
		} else {
			state.Put("iso_mounted", false)
		}
	}

	ui.Sayf("Exporting vApp as template to catalog: %s", s.Config.Catalog)

	// Get or create the catalog
	catalog, err := d.GetCatalog(s.Config.Catalog)
	if err != nil {
		if !s.Config.CreateCatalog {
			state.Put("error", fmt.Errorf("error getting catalog %s: %w", s.Config.Catalog, err))
			return multistep.ActionHalt
		}

		// Create the catalog
		ui.Sayf("Catalog '%s' not found, creating...", s.Config.Catalog)
		adminCatalog, err := d.CreateCatalog(s.Config.Catalog, "Created by Packer")
		if err != nil {
			state.Put("error", fmt.Errorf("error creating catalog %s: %w", s.Config.Catalog, err))
			return multistep.ActionHalt
		}

		// Get the regular catalog reference
		catalog, err = d.GetCatalog(s.Config.Catalog)
		if err != nil {
			state.Put("error", fmt.Errorf("error getting newly created catalog %s: %w", s.Config.Catalog, err))
			return multistep.ActionHalt
		}
		_ = adminCatalog // used only for creation
		ui.Sayf("Catalog '%s' created successfully", s.Config.Catalog)
	}

	// Check if template already exists and determine capture name
	captureName := s.Config.TemplateName
	needsRename := false

	existingItem, err := catalog.GetCatalogItemByName(s.Config.TemplateName, false)
	if err == nil && existingItem != nil {
		if !s.Config.Overwrite {
			state.Put("error", fmt.Errorf("template '%s' already exists in catalog '%s'. Set overwrite=true to replace it",
				s.Config.TemplateName, s.Config.Catalog))
			return multistep.ActionHalt
		}
		// Use a temporary name - will rename after capture completes
		captureName = fmt.Sprintf("%s-packer-%d", s.Config.TemplateName, time.Now().UnixNano())
		needsRename = true
		ui.Sayf("Template '%s' exists, will capture as '%s' first then replace", s.Config.TemplateName, captureName)
	}

	// Create vApp template from vApp
	vappRef := vapp.(*govcd.VApp)
	description := s.Config.Description
	if description == "" {
		description = fmt.Sprintf("Packer-built template from %s", vappRef.VApp.Name)
	}

	ui.Sayf("Creating vApp template: %s (this may take a few minutes...)", captureName)
	captureParams := &types.CaptureVAppParams{
		Name:        captureName,
		Description: description,
		Source: &types.Reference{
			HREF: vappRef.VApp.HREF,
		},
		CustomizationSection: types.CaptureVAppParamsCustomizationSection{
			Info:                   "CustomizeOnInstantiate Settings",
			CustomizeOnInstantiate: true,
		},
	}

	_, err = catalog.CaptureVappTemplate(captureParams)
	if err != nil {
		state.Put("error", fmt.Errorf("error capturing vApp as template: %w", err))
		return multistep.ActionHalt
	}

	ui.Sayf("vApp template '%s' captured successfully", captureName)

	// Wait for template to reach status 8 (resolved and powered off)
	ui.Say("Waiting for vApp template to be ready (status 8)...")
	statusTimeout := time.After(templateStatusTimeout)
	for {
		template, err := catalog.GetVAppTemplateByName(captureName)
		if err != nil {
			ui.Sayf("Warning: error checking template status: %s", err)
			time.Sleep(templateStatusPollDelay)
			continue
		}

		if template.VAppTemplate.Status == 8 {
			ui.Say("vApp template is ready")
			break
		}

		ui.Sayf("Template status: %d (waiting for 8)...", template.VAppTemplate.Status)

		select {
		case <-statusTimeout:
			state.Put("error", fmt.Errorf("vApp template did not reach ready state within %v", templateStatusTimeout))
			return multistep.ActionHalt
		case <-time.After(templateStatusPollDelay):
			// Continue polling
		}
	}

	// If we used a temp name, delete old and rename
	if needsRename {
		ui.Sayf("Deleting old template '%s'...", s.Config.TemplateName)
		oldItem, err := catalog.GetCatalogItemByName(s.Config.TemplateName, false)
		if err == nil && oldItem != nil {
			if err := oldItem.Delete(); err != nil && !strings.Contains(err.Error(), "not found") {
				state.Put("error", fmt.Errorf("error deleting old template '%s': %w", s.Config.TemplateName, err))
				return multistep.ActionHalt
			}

			// Wait for deletion
			deleteTimeout := time.After(templateDeleteTimeout)
			for {
				deletedItem, err := catalog.GetCatalogItemByName(s.Config.TemplateName, false)
				if err != nil || deletedItem == nil {
					ui.Say("Old template deleted successfully")
					break
				}

				select {
				case <-deleteTimeout:
					state.Put("error", fmt.Errorf("old template was not deleted within %v", templateDeleteTimeout))
					return multistep.ActionHalt
				case <-time.After(10 * time.Second):
					// Continue polling
				}
			}
		}

		// Rename new template
		ui.Sayf("Renaming template '%s' to '%s'...", captureName, s.Config.TemplateName)
		newTemplate, err := catalog.GetVAppTemplateByName(captureName)
		if err != nil {
			state.Put("error", fmt.Errorf("error getting new template for rename: %w", err))
			return multistep.ActionHalt
		}

		newTemplate.VAppTemplate.Name = s.Config.TemplateName
		_, err = newTemplate.Update()
		if err != nil {
			state.Put("error", fmt.Errorf("error renaming template to '%s': %w", s.Config.TemplateName, err))
			return multistep.ActionHalt
		}
		ui.Sayf("Template renamed successfully to '%s'", s.Config.TemplateName)
	}

	ui.Sayf("vApp template '%s' created successfully in catalog '%s'", s.Config.TemplateName, s.Config.Catalog)

	return multistep.ActionContinue
}

func (s *StepExportToCatalog) Cleanup(state multistep.StateBag) {
	// No cleanup needed - we want to keep the exported template
}
