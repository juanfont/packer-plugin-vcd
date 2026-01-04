// Copyright 2025 Juan Font
// BSD-3-Clause

package iso

import (
	"context"

	"github.com/hashicorp/hcl/v2/hcldec"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	"github.com/hashicorp/packer-plugin-sdk/multistep/commonsteps"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/juanfont/packer-plugin-vcd/builder/vcd/common"
	"github.com/juanfont/packer-plugin-vcd/builder/vcd/driver"
)

type Builder struct {
	config Config
	runner multistep.Runner
}

// ConfigSpec returns an HCL2 object specification based on the Builder's configuration mapping.
func (b *Builder) ConfigSpec() hcldec.ObjectSpec {
	return b.config.FlatMapstructure().HCL2Spec()
}

// Prepare processes the given raw inputs, validates the configuration, and returns warnings or errors if any occur.
func (b *Builder) Prepare(raws ...interface{}) ([]string, []string, error) {
	warnings, errs := b.config.Prepare(raws...)
	if errs != nil {
		return nil, warnings, errs
	}

	return nil, warnings, nil
}

// Run executes the build process steps for the `Builder`, leveraging the provided context, UI, and lifecycle hook.
// It initializes state, configures steps sequentially, and manages interactions with the virtual machine driver.
// Returns a finalized artifact or an error if the build process fails.
func (b *Builder) Run(ctx context.Context, ui packersdk.Ui, hook packersdk.Hook) (packersdk.Artifact, error) {
	state := new(multistep.BasicStateBag)
	state.Put("debug", b.config.PackerDebug)
	state.Put("hook", hook)
	state.Put("ui", ui)

	var steps []multistep.Step

	steps = append(steps,
		// Step 1: Connect to VCD
		&common.StepConnect{
			Config: &b.config.ConnectConfig,
		},

		// Step 2: Download ISO locally (using Packer SDK)
		&commonsteps.StepDownload{
			Checksum:    b.config.ISOChecksum,
			Description: "ISO",
			Extension:   b.config.TargetExtension,
			ResultKey:   "iso_path",
			TargetPath:  b.config.TargetPath,
			Url:         b.config.ISOUrls,
		},

		// Step 3: Create temporary catalog (or use existing)
		&common.StepCreateTempCatalog{
			Config:  &b.config.CatalogConfig,
			VDCName: b.config.LocationConfig.VDC,
		},

		// Step 4: Upload ISO to catalog (with caching support)
		&common.StepUploadISO{
			CacheISO:       b.config.CatalogConfig.CacheISO,
			CacheOverwrite: b.config.CatalogConfig.CacheOverwrite,
		},

		// Step 5: Resolve or create vApp
		&common.StepResolveVApp{
			VDCName:     b.config.LocationConfig.VDC,
			VAppName:    b.config.LocationConfig.VApp,
			NetworkName: b.config.LocationConfig.Network,
			CreateVApp:  b.config.LocationConfig.CreateVApp,
		},

		// Step 6: Create empty VM
		&StepCreateVM{
			VMName:           b.config.LocationConfig.VMName,
			Description:      b.config.CreateConfig.Description,
			StorageProfile:   b.config.LocationConfig.StorageProfile,
			Network:          b.config.LocationConfig.Network,
			IPAllocationMode: b.config.LocationConfig.IPAllocationMode,
			GuestOSType:      b.config.CreateConfig.GuestOSType,
			Firmware:         b.config.HardwareConfig.Firmware,
			DiskSizeMB:       b.config.CreateConfig.DiskSizeMB,
		},

		// Step 7: Configure hardware (CPU, memory)
		&StepHardware{
			Config: &b.config.HardwareConfig,
		},

		// Step 8: Mount ISO to VM
		&common.StepMountISO{},

		// Step 9: Power on VM
		&common.StepRun{
			Config: &b.config.RunConfig,
		},

		// Step 10: Boot command via WMKS console
		&common.StepBootCommand{
			Config: &b.config.BootCommandConfig,
			VMName: b.config.LocationConfig.VMName,
			Ctx:    b.config.ctx,
		},

		// TODO: Future steps for Phase 3:
		// - HTTP server for kickstart files
		// - Provisioners (SSH/WinRM communicator)

		// Step 11: Shutdown VM
		&common.StepShutdown{
			Config: &b.config.ShutdownConfig,
		},

		// Step 12: Export to catalog (optional)
		&common.StepExportToCatalog{
			Config: b.config.ExportToCatalog,
		},
	)

	b.runner = commonsteps.NewRunnerWithPauseFn(steps, b.config.PackerConfig, ui, state)
	b.runner.Run(ctx, state)

	if rawErr, ok := state.GetOk("error"); ok {
		return nil, rawErr.(error)
	}

	if _, ok := state.GetOk("vm"); !ok {
		return nil, nil
	}

	vm := state.Get("vm").(driver.VirtualMachine)
	artifact := &common.Artifact{
		Name:     b.config.LocationConfig.VMName,
		Location: b.config.LocationConfig,
		VM:       vm,
		StateData: map[string]interface{}{
			"iso_path":     state.Get("iso_path"),
			"catalog_name": state.Get("catalog_name"),
			"vapp_name":    state.Get("vapp_name"),
		},
	}

	if b.config.Export != nil {
		artifact.Outconfig = &b.config.Export.OutputDir.OutputDir
	}

	return artifact, nil
}
