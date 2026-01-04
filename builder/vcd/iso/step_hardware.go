package iso

import (
	"context"
	"fmt"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/juanfont/packer-plugin-vcd/builder/vcd/common"
	"github.com/juanfont/packer-plugin-vcd/builder/vcd/driver"
)

type StepHardware struct {
	Config *common.HardwareConfig
}

func (s *StepHardware) Run(_ context.Context, state multistep.StateBag) multistep.StepAction {
	ui := state.Get("ui").(packersdk.Ui)
	vm := state.Get("vm").(driver.VirtualMachine)

	if s.Config.CPUs > 0 {
		ui.Sayf("Configuring CPU: %d CPUs, %d cores per socket", s.Config.CPUs, s.Config.CoresPerSocket)

		coresPerSocket := int(s.Config.CoresPerSocket)
		if coresPerSocket == 0 {
			coresPerSocket = 1
		}

		err := vm.ChangeCPU(int(s.Config.CPUs), coresPerSocket)
		if err != nil {
			state.Put("error", fmt.Errorf("error configuring CPU: %w", err))
			return multistep.ActionHalt
		}
	}

	if s.Config.Memory > 0 {
		ui.Sayf("Configuring memory: %d MB", s.Config.Memory)
		err := vm.ChangeMemory(s.Config.Memory)
		if err != nil {
			state.Put("error", fmt.Errorf("error configuring memory: %w", err))
			return multistep.ActionHalt
		}
	}

	// Refresh VM state after hardware changes
	if err := vm.Refresh(); err != nil {
		state.Put("error", fmt.Errorf("error refreshing VM after hardware changes: %w", err))
		return multistep.ActionHalt
	}

	ui.Say("Hardware configuration complete")
	return multistep.ActionContinue
}

func (s *StepHardware) Cleanup(state multistep.StateBag) {
	// Nothing to clean up
}
