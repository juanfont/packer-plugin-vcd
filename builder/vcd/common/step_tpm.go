package common

import (
	"context"
	"fmt"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/juanfont/packer-plugin-vcd/builder/vcd/driver"
)

// StepConfigureTPM enables the virtual TPM on the VM if configured.
// This must run after VM creation but before the VM is powered on.
type StepConfigureTPM struct {
	Enabled bool
}

func (s *StepConfigureTPM) Run(_ context.Context, state multistep.StateBag) multistep.StepAction {
	if !s.Enabled {
		return multistep.ActionContinue
	}

	ui := state.Get("ui").(packersdk.Ui)
	vm := state.Get("vm").(driver.VirtualMachine)

	ui.Say("Enabling virtual TPM...")

	if err := vm.SetTPM(true); err != nil {
		state.Put("error", fmt.Errorf("error enabling TPM: %w", err))
		return multistep.ActionHalt
	}

	ui.Say("Virtual TPM enabled successfully")
	return multistep.ActionContinue
}

func (s *StepConfigureTPM) Cleanup(state multistep.StateBag) {
	// No cleanup needed - TPM is part of the VM
}
