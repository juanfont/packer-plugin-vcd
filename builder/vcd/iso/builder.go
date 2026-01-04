// Copyright 2025 Juan Font
// BSD-3-Clause

package iso

import (
	"context"
	"fmt"

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
func (b *Builder) ConfigSpec() hcldec.ObjectSpec { return b.config.FlatMapstructure().HCL2Spec() }

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
		&common.StepConnect{
			Config: &b.config.ConnectConfig,
		},
		// &common.StepDownload{
		// 	DownloadStep: &commonsteps.StepDownload{
		// 		Checksum:    b.config.ISOChecksum,
		// 		Description: "ISO",
		// 		Extension:   b.config.TargetExtension,
		// 		ResultKey:   "iso_path",
		// 		TargetPath:  b.config.TargetPath,
		// 		Url:         b.config.ISOUrls,
		// 	},
		// 	Url:                  b.config.ISOUrls,
		// 	ResultKey:            "iso_path",
		// 	Datastore:            b.config.Datastore,
		// 	Host:                 b.config.Host,
		// 	LocalCacheOverwrite:  b.config.LocalCacheOverwrite,
		// 	RemoteCacheOverwrite: b.config.RemoteCacheOverwrite || b.config.LocalCacheOverwrite,
		// 	RemoteCacheDatastore: b.config.RemoteCacheDatastore,
		// 	RemoteCachePath:      b.config.RemoteCachePath,
		// },
	)

	b.runner = commonsteps.NewRunnerWithPauseFn(steps, b.config.PackerConfig, ui, state)
	b.runner.Run(ctx, state)

	if rawErr, ok := state.GetOk("error"); ok {
		return nil, rawErr.(error)
	}

	if _, ok := state.GetOk("vm"); !ok {
		return nil, nil
	}

	vm := state.Get("vm").(*driver.VirtualMachineDriver)
	artifact := &common.Artifact{
		// Name:                 b.config.VMName,
		// Datacenter:           vm.Datacenter(),
		// Location:             b.config.LocationConfig,
		// ContentLibraryConfig: b.config.ContentLibraryDestinationConfig,
		// VM:                   vm,
		// StateData: map[string]interface{}{
		// 	"generated_data": state.Get("generated_data"),
		// 	"metadata":       state.Get("metadata"),
		// 	"SourceImageURL": state.Get("SourceImageURL"),
		// 	"iso_path":       state.Get("iso_path"),
		// },
	}

	fmt.Println("artifact", vm)

	if b.config.Export != nil {
		// artifact.Outconfig = &b.config.Export.OutputDir
	}

	return artifact, nil
}
