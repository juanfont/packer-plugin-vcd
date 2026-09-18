package common

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/juanfont/packer-plugin-vcd/builder/vcd/driver"
	"github.com/vmware/go-vcloud-director/v3/govcd"
	"github.com/vmware/go-vcloud-director/v3/types/v56"
)

//go:generate packer-sdc struct-markdown
//go:generate packer-sdc mapstructure-to-hcl2 -type CatalogConfig

// CatalogConfig defines configuration for the ISO source catalog during build.
// This catalog is used to upload and store the ISO image used to boot the VM.
// For the output catalog where the final vApp template is exported, see ExportToCatalogConfig.
type CatalogConfig struct {
	// The name of a persistent catalog to use for ISO upload. It is created on
	// first use if it does not exist, and never deleted by the plugin.
	// If not set, a temporary catalog will be created and deleted after the build.
	// Using a persistent catalog enables ISO caching across builds.
	// This catalog is separate from the output catalog where the final vApp template is exported.
	ISOCatalog string `mapstructure:"iso_catalog"`

	// Prefix for temporary catalog names when creating a new catalog.
	// Only used when iso_catalog is not set.
	// Defaults to "packer-".
	TempCatalogPrefix string `mapstructure:"temp_catalog_prefix"`

	// If true and using an existing catalog (iso_catalog), check if the ISO already exists
	// before uploading. This enables reusing ISOs across multiple builds.
	// Defaults to true when iso_catalog is specified.
	// An ISO modified by cd_content/cd_files is never reused: it embeds per-build
	// values (such as the VM's IP), so it is uploaded for every build and deleted
	// from the catalog afterwards.
	CacheISO bool `mapstructure:"cache_iso"`

	// If true, overwrite existing cached ISO even if it exists in the catalog.
	// Defaults to false.
	CacheOverwrite bool `mapstructure:"cache_overwrite"`
}

func (c *CatalogConfig) Prepare() []error {
	var errs []error

	if c.TempCatalogPrefix == "" {
		c.TempCatalogPrefix = "packer-"
	}

	// Default to caching ISOs when using an existing catalog
	if c.ISOCatalog != "" && !c.CacheOverwrite {
		c.CacheISO = true
	}

	return errs
}

type StepCreateTempCatalog struct {
	Config         *CatalogConfig
	VDCName        string // VDC name to get storage profile from for catalog storage
	StorageProfile string // Optional storage profile name. If empty, uses VDC default.
}

func (s *StepCreateTempCatalog) Run(_ context.Context, state multistep.StateBag) multistep.StepAction {
	ui := state.Get("ui").(packersdk.Ui)
	d := state.Get("driver").(driver.Driver)

	// If a catalog is specified, use it, creating it on first use
	if s.Config.ISOCatalog != "" {
		ui.Sayf("Using ISO catalog: %s", s.Config.ISOCatalog)
		catalog, created, err := getOrCreateCatalog(d, s.Config.ISOCatalog, func() (*types.Reference, error) {
			return s.resolveStorageProfile(state, d, ui)
		})
		if err != nil {
			state.Put("error", err)
			return multistep.ActionHalt
		}
		if created {
			ui.Sayf("ISO catalog %s did not exist, created it (persistent, never deleted by Packer)", s.Config.ISOCatalog)
		}
		state.Put("catalog", catalog)
		state.Put("catalog_name", s.Config.ISOCatalog)
		state.Put("temp_catalog", false)
		return multistep.ActionContinue
	}

	// Create a temporary catalog
	catalogName := fmt.Sprintf("%s%d", s.Config.TempCatalogPrefix, time.Now().UnixNano())
	ui.Sayf("Creating temporary catalog: %s", catalogName)

	storageProfileRef, err := s.resolveStorageProfile(state, d, ui)
	if err != nil {
		state.Put("error", err)
		return multistep.ActionHalt
	}

	adminCatalog, err := d.CreateCatalogWithStorageProfile(catalogName, "Temporary catalog for Packer ISO build", storageProfileRef)
	if err != nil {
		state.Put("error", fmt.Errorf("error creating temporary catalog: %w", err))
		return multistep.ActionHalt
	}

	// Get the regular catalog reference for media operations
	catalog, err := d.GetCatalog(catalogName)
	if err != nil {
		// Try to clean up the admin catalog we just created
		_ = d.DeleteCatalog(adminCatalog)
		state.Put("error", fmt.Errorf("error getting created catalog: %w", err))
		return multistep.ActionHalt
	}

	state.Put("catalog", catalog)
	state.Put("admin_catalog", adminCatalog)
	state.Put("catalog_name", catalogName)
	state.Put("temp_catalog", true)

	ui.Sayf("Temporary catalog created: %s", catalogName)
	return multistep.ActionContinue
}

// resolveStorageProfile picks the storage profile a new catalog should live on:
// the configured one, else the VDC default. It also stores the VDC in state for
// later steps.
func (s *StepCreateTempCatalog) resolveStorageProfile(state multistep.StateBag, d driver.Driver, ui packersdk.Ui) (*types.Reference, error) {
	if s.VDCName == "" {
		return nil, nil
	}
	vdc, err := d.GetVdc(s.VDCName)
	if err != nil {
		return nil, fmt.Errorf("error getting VDC %s for storage profile: %w", s.VDCName, err)
	}
	state.Put("vdc", vdc)

	if s.StorageProfile != "" {
		sp, err := vdc.FindStorageProfileReference(s.StorageProfile)
		if err != nil {
			return nil, fmt.Errorf("error finding storage profile %s: %w", s.StorageProfile, err)
		}
		ui.Sayf("Using specified storage profile: %s", sp.Name)
		return &sp, nil
	}
	if vdc.Vdc.VdcStorageProfiles != nil && len(vdc.Vdc.VdcStorageProfiles.VdcStorageProfile) > 0 {
		ref := vdc.Vdc.VdcStorageProfiles.VdcStorageProfile[0]
		ui.Sayf("Using VDC default storage profile: %s", ref.Name)
		return ref, nil
	}
	return nil, nil
}

// catalogStore is the slice of driver.Driver that getOrCreateCatalog needs.
type catalogStore interface {
	GetCatalog(name string) (*govcd.Catalog, error)
	CreateCatalogWithStorageProfile(name, description string, storageProfileRef *types.Reference) (*govcd.AdminCatalog, error)
}

// getOrCreateCatalog returns the named catalog, creating it when it does not
// exist. Only a "not found" lookup triggers creation; any other error (auth,
// network) is returned as is. Builds often start in parallel, so losing the
// creation race to another build is expected: whatever the create call says,
// the catalog is looked up again and used if it is there.
func getOrCreateCatalog(store catalogStore, name string, storageProfile func() (*types.Reference, error)) (catalog *govcd.Catalog, created bool, err error) {
	catalog, err = store.GetCatalog(name)
	if err == nil {
		return catalog, false, nil
	}
	if !govcd.ContainsNotFound(err) {
		return nil, false, fmt.Errorf("error getting catalog %s: %w", name, err)
	}

	spRef, err := storageProfile()
	if err != nil {
		return nil, false, err
	}
	_, createErr := store.CreateCatalogWithStorageProfile(name, "ISO cache for Packer builds", spRef)

	catalog, err = store.GetCatalog(name)
	if err != nil {
		if createErr != nil {
			return nil, false, fmt.Errorf("catalog %s does not exist and could not be created: %w", name, createErr)
		}
		return nil, false, fmt.Errorf("error getting created catalog %s: %w", name, err)
	}
	return catalog, createErr == nil, nil
}

func (s *StepCreateTempCatalog) Cleanup(state multistep.StateBag) {
	ui := state.Get("ui").(packersdk.Ui)
	d := state.Get("driver").(driver.Driver)

	tempCatalog, ok := state.GetOk("temp_catalog")
	if !ok || !tempCatalog.(bool) {
		return
	}

	adminCatalog, ok := state.GetOk("admin_catalog")
	if !ok {
		return
	}

	catalogName, _ := state.GetOk("catalog_name")
	ui.Sayf("Deleting temporary catalog: %s (waiting for completion)...", catalogName)

	err := d.DeleteCatalog(adminCatalog.(*govcd.AdminCatalog))
	if err != nil {
		ui.Errorf("Error deleting temporary catalog: %s", err)
	} else {
		ui.Say("Temporary catalog deleted successfully")
	}
}
