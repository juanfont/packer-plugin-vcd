package common

import (
	"context"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/juanfont/packer-plugin-vcd/builder/vcd/driver"
)

type RunConfig struct{}

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
	vm := state.Get("vm").(*driver.VirtualMachineDriver)

	// TODO(juan): Implement this
	// https://github.com/hashicorp/packer-plugin-vsphere/blob/main/builder/vsphere/common/step_run.go#L18

	return multistep.ActionContinue
}

func (s *StepRun) Cleanup(state multistep.StateBag) {
	ui := state.Get("ui").(packersdk.Ui)
	vm := state.Get("vm").(*driver.VirtualMachineDriver)

	_, cancelled := state.GetOk(multistep.StateCancelled)
	_, halted := state.GetOk(multistep.StateHalted)
	if !cancelled && !halted {
		return
	}

	ui.Say("Powering off virtual machine...")

	err := vm.PowerOff()
	if err != nil {
		ui.Errorf("%s", err)
	}
}
