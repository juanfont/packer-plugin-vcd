package iso

import (
	"context"
	"fmt"
	"net/url"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/juanfont/packer-plugin-vcd/builder/vcd/common"
	"github.com/juanfont/packer-plugin-vcd/builder/vcd/driver"
	"github.com/vmware/go-vcloud-director/v3/govcd"
)

type StepHardware struct {
	Config *common.HardwareConfig
}

func (s *StepHardware) Run(_ context.Context, state multistep.StateBag) multistep.StepAction {
	ui := state.Get("ui").(packersdk.Ui)
	vm := state.Get("vm").(driver.VirtualMachine)
	d := state.Get("driver").(driver.Driver)

	// Check if using sizing policy instead of manual CPU/memory
	if s.Config.VMSizingPolicy != "" {
		ui.Sayf("Applying VM sizing policy: %s", s.Config.VMSizingPolicy)

		vdc := state.Get("vdc").(*govcd.Vdc)
		client := d.GetClient()

		// Get all assigned sizing policies from VDC
		sizingPolicies, err := client.GetAllAssignedVdcComputePoliciesV2(vdc.Vdc.ID, url.Values{})
		if err != nil {
			state.Put("error", fmt.Errorf("error getting sizing policies: %w", err))
			return multistep.ActionHalt
		}

		// Find the policy by name
		sizingPolicy, err := driver.GetVMSizingPolicyByName(sizingPolicies, s.Config.VMSizingPolicy)
		if err != nil {
			state.Put("error", fmt.Errorf("VM sizing policy '%s' not found in VDC", s.Config.VMSizingPolicy))
			return multistep.ActionHalt
		}

		// Preserve existing placement policy
		govcdVM := vm.GetVM()
		placementPolicy := ""
		if govcdVM.VM.ComputePolicy != nil && govcdVM.VM.ComputePolicy.VmPlacementPolicy != nil && govcdVM.VM.ComputePolicy.VmPlacementPolicy.ID != "" {
			placementPolicy = govcdVM.VM.ComputePolicy.VmPlacementPolicy.ID
		}

		// Apply sizing policy
		_, err = govcdVM.UpdateComputePolicyV2(sizingPolicy.VdcComputePolicyV2.ID, placementPolicy, "")
		if err != nil {
			state.Put("error", fmt.Errorf("error applying sizing policy: %w", err))
			return multistep.ActionHalt
		}

		ui.Say("VM sizing policy applied successfully")
	} else {
		// Manual CPU/memory configuration
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
