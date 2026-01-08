package common

import (
	"context"
	"fmt"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/juanfont/packer-plugin-vcd/builder/vcd/driver"
)

// StepMountFloppy mounts the uploaded floppy image to the VM
// This must happen after VM creation but before power on
type StepMountFloppy struct{}

func (s *StepMountFloppy) Run(_ context.Context, state multistep.StateBag) multistep.StepAction {
	floppyMediaName, ok := state.GetOk("floppy_media_name")
	if !ok {
		// No floppy to mount
		return multistep.ActionContinue
	}

	floppyCatalogName, ok := state.GetOk("floppy_catalog_name")
	if !ok {
		return multistep.ActionContinue
	}

	ui := state.Get("ui").(packersdk.Ui)
	vm := state.Get("vm").(driver.VirtualMachine)

	ui.Sayf("Mounting floppy image %s to VM...", floppyMediaName.(string))

	err := vm.MountFloppy(floppyCatalogName.(string), floppyMediaName.(string))
	if err != nil {
		state.Put("error", fmt.Errorf("error mounting floppy: %w", err))
		return multistep.ActionHalt
	}

	ui.Say("Floppy image mounted successfully")
	return multistep.ActionContinue
}

func (s *StepMountFloppy) Cleanup(state multistep.StateBag) {
	// Floppy media cleanup is handled by StepUploadFloppy
}
