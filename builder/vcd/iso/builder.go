// Copyright 2025 Juan Font
// BSD-3-Clause

package iso

import (
	"context"

	"github.com/hashicorp/hcl/v2/hcldec"
	"github.com/hashicorp/packer-plugin-sdk/communicator"
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

	// Determine if we need VCD to assign IP (POOL mode) because we have cd_content
	// that needs template variables filled with the actual assigned IP
	hasCDContent := len(b.config.CDConfig.CDContent) > 0 || len(b.config.CDConfig.CDFiles) > 0
	needsPoolAllocation := b.config.LocationConfig.AutoDiscoverIP && hasCDContent

	// Determine IP allocation mode
	ipAllocationMode := b.config.LocationConfig.IPAllocationMode
	if needsPoolAllocation {
		// Use POOL mode - let VCD assign IP, then query it after VM creation
		ipAllocationMode = "POOL"
	}

	var steps []multistep.Step

	// Common initial steps
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

		// Step 3: Discover HTTP IP for preseed/kickstart server
		&common.StepHTTPIPDiscover{
			HTTPIP:        b.config.HTTPConfig.HTTPAddress,
			HTTPInterface: b.config.HTTPConfig.HTTPInterface,
			TargetHost:    b.config.ConnectConfig.Host,
		},

		// Step 4: Start HTTP server for preseed/kickstart files
		commonsteps.HTTPServerFromHTTPConfig(&b.config.HTTPConfig),
	)

	if needsPoolAllocation {
		// POOL allocation flow: Create VM first, query IP, then modify ISO
		// This ensures we get an IP that VCD has actually allocated
		steps = append(steps,
			// Step 5: Create temporary catalog
			&common.StepCreateTempCatalog{
				Config:         &b.config.CatalogConfig,
				VDCName:        b.config.LocationConfig.VDC,
				StorageProfile: b.config.LocationConfig.StorageProfile,
			},

			// Step 6: Resolve or create vApp
			&common.StepResolveVApp{
				VDCName:     b.config.LocationConfig.VDC,
				VAppName:    b.config.LocationConfig.VApp,
				NetworkName: b.config.LocationConfig.Network,
				CreateVApp:  b.config.LocationConfig.CreateVApp,
			},

			// Step 7: Create VM with POOL allocation (VCD assigns IP)
			&StepCreateVM{
				VMName:           b.config.LocationConfig.VMName,
				Description:      b.config.CreateConfig.Description,
				StorageProfile:   b.config.LocationConfig.StorageProfile,
				Network:          b.config.LocationConfig.Network,
				IPAllocationMode: ipAllocationMode,
				GuestOSType:      b.config.CreateConfig.GuestOSType,
				Firmware:         b.config.HardwareConfig.Firmware,
				HardwareVersion:  b.config.HardwareConfig.HardwareVersion,
				DiskSizeMB:       b.config.CreateConfig.DiskSizeMB,
			},

			// Step 8: Configure hardware (CPU, memory)
			&StepHardware{
				Config: &b.config.HardwareConfig,
			},

			// Step 9: Configure boot options (delay, EFI secure boot)
			// Must be before TPM - TPM requires EFI firmware
			&common.StepConfigureBootOptions{
				BootDelay: b.config.HardwareConfig.BootDelay,
				Firmware:  b.config.HardwareConfig.Firmware,
			},

			// Step 10: Configure TPM (if enabled)
			&common.StepConfigureTPM{
				Enabled: b.config.HardwareConfig.VTPMEnabled,
			},

			// Step 11: Query the IP that VCD assigned to the VM
			&common.StepQueryVMIP{
				VDCName:         b.config.LocationConfig.VDC,
				NetworkName:     b.config.LocationConfig.Network,
				OverrideGateway: b.config.LocationConfig.VMGateway,
				OverrideDNS:     b.config.LocationConfig.VMDNS,
			},

			// Step 11: NOW modify ISO with the actual assigned IP
			&common.StepModifyISO{
				Config: &b.config.CDConfig,
			},

			// Step 12: Upload modified ISO to catalog
			&common.StepUploadISO{
				CacheISO:       false, // Don't cache modified ISOs
				CacheOverwrite: false,
			},

			// Step 13: Mount ISO to VM
			&common.StepMountISO{},
		)
	} else {
		// Standard flow: Discover/set IP early, modify ISO, then create VM
		steps = append(steps,
			// Step 5: Discover or set IP early (for cd_content templates)
			&common.StepDiscoverIP{
				VDCName:         b.config.LocationConfig.VDC,
				NetworkName:     b.config.LocationConfig.Network,
				AutoDiscover:    b.config.LocationConfig.AutoDiscoverIP,
				ManualIP:        b.config.LocationConfig.VMIPAddress,
				OverrideGateway: b.config.LocationConfig.VMGateway,
				OverrideDNS:     b.config.LocationConfig.VMDNS,
			},

			// Step 6: Modify ISO (if cd_content/cd_files specified)
			&common.StepModifyISO{
				Config: &b.config.CDConfig,
			},

			// Step 7: Create temporary catalog
			&common.StepCreateTempCatalog{
				Config:         &b.config.CatalogConfig,
				VDCName:        b.config.LocationConfig.VDC,
				StorageProfile: b.config.LocationConfig.StorageProfile,
			},

			// Step 8: Upload ISO to catalog
			&common.StepUploadISO{
				CacheISO:       b.config.CatalogConfig.CacheISO,
				CacheOverwrite: b.config.CatalogConfig.CacheOverwrite,
			},

			// Step 9: Resolve or create vApp
			&common.StepResolveVApp{
				VDCName:     b.config.LocationConfig.VDC,
				VAppName:    b.config.LocationConfig.VApp,
				NetworkName: b.config.LocationConfig.Network,
				CreateVApp:  b.config.LocationConfig.CreateVApp,
			},

			// Step 10: Create VM
			&StepCreateVM{
				VMName:           b.config.LocationConfig.VMName,
				Description:      b.config.CreateConfig.Description,
				StorageProfile:   b.config.LocationConfig.StorageProfile,
				Network:          b.config.LocationConfig.Network,
				IPAllocationMode: ipAllocationMode,
				GuestOSType:      b.config.CreateConfig.GuestOSType,
				Firmware:         b.config.HardwareConfig.Firmware,
				HardwareVersion:  b.config.HardwareConfig.HardwareVersion,
				DiskSizeMB:       b.config.CreateConfig.DiskSizeMB,
			},

			// Step 11: Configure hardware (CPU, memory)
			&StepHardware{
				Config: &b.config.HardwareConfig,
			},

			// Step 12: Configure boot options (delay, EFI secure boot)
			// Must be before TPM - TPM requires EFI firmware
			&common.StepConfigureBootOptions{
				BootDelay: b.config.HardwareConfig.BootDelay,
				Firmware:  b.config.HardwareConfig.Firmware,
			},

			// Step 13: Configure TPM (if enabled)
			&common.StepConfigureTPM{
				Enabled: b.config.HardwareConfig.VTPMEnabled,
			},

			// Step 14: Mount ISO to VM
			&common.StepMountISO{},
		)
	}

	// Common final steps for both flows
	steps = append(steps,
		// Power on VM (with IP conflict retry logic)
		&common.StepRun{
			Config:      &b.config.RunConfig,
			VDCName:     b.config.LocationConfig.VDC,
			NetworkName: b.config.LocationConfig.Network,
		},

		// Boot command via WMKS console
		&common.StepBootCommand{
			Config: &b.config.BootCommandConfig,
			VMName: b.config.LocationConfig.VMName,
			Ctx:    b.config.ctx,
		},

		// Wait for VM to get IP address (for communicator)
		&common.StepWaitForIP{
			Config: &b.config.WaitIpConfig,
		},

		// Connect to VM via SSH/WinRM
		&communicator.StepConnect{
			Config:    &b.config.Comm,
			Host:      common.CommHost(b.config.Comm.Host()),
			SSHConfig: b.config.Comm.SSHConfigFunc(),
		},

		// Run provisioners
		&commonsteps.StepProvision{},

		// Shutdown VM
		&common.StepShutdown{
			Config:   &b.config.ShutdownConfig,
			CommType: b.config.Comm.Type,
		},

		// Export to catalog (optional)
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
