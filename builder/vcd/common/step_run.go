package common

import (
	"context"
	"fmt"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/juanfont/packer-plugin-vcd/builder/vcd/driver"
)

type RunConfig struct {
	// The priority of boot devices. Defaults to `disk,cdrom`.
	//
	// The available boot devices are: `floppy`, `cdrom`, `ethernet`, and
	// `disk`.
	//
	// -> **Note:** If not set, the boot order is temporarily set to
	// `disk,cdrom` for the duration of the build and then cleared upon
	// build completion.
	BootOrder string `mapstructure:"boot_order"`
}

type StepRun struct {
	Config   *RunConfig
	SetOrder bool
}

func (s *StepRun) Run(_ context.Context, state multistep.StateBag) multistep.StepAction {
	ui := state.Get("ui").(packersdk.Ui)
	vm := state.Get("vm").(driver.VirtualMachine)

	ui.Say("Powering on virtual machine...")

	err := vm.PowerOn()
	if err != nil {
		err = fmt.Errorf("error powering on VM: %w", err)
		state.Put("error", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	ui.Say("Virtual machine powered on.")
	return multistep.ActionContinue
}

func (s *StepRun) Cleanup(state multistep.StateBag) {
	ui := state.Get("ui").(packersdk.Ui)

	vmRaw, ok := state.GetOk("vm")
	if !ok {
		return
	}
	vm := vmRaw.(driver.VirtualMachine)

	_, cancelled := state.GetOk(multistep.StateCancelled)
	_, halted := state.GetOk(multistep.StateHalted)
	if !cancelled && !halted {
		return
	}

	ui.Say("Powering off virtual machine...")

	err := vm.PowerOff()
	if err != nil {
		ui.Errorf("Error powering off VM: %s", err)
	}
}
