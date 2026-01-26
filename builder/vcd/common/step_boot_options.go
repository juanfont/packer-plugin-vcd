package common

import (
	"context"
	"fmt"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/juanfont/packer-plugin-vcd/builder/vcd/driver"
)

// StepConfigureBootOptions configures boot delay and EFI secure boot.
// This must run after VM creation but before the VM is powered on.
type StepConfigureBootOptions struct {
	// BootDelay in seconds (will be converted to milliseconds)
	BootDelay int
	// Firmware setting - if "efi-secure", enables EFI secure boot
	Firmware string
}

func (s *StepConfigureBootOptions) Run(_ context.Context, state multistep.StateBag) multistep.StepAction {
	// Check if we have anything to configure
	efiSecureBoot := s.Firmware == "efi-secure"
	if s.BootDelay == 0 && !efiSecureBoot {
		return multistep.ActionContinue
	}

	ui := state.Get("ui").(packersdk.Ui)
	vm := state.Get("vm").(driver.VirtualMachine)

	// Convert seconds to milliseconds
	bootDelayMs := s.BootDelay * 1000

	if bootDelayMs > 0 && efiSecureBoot {
		ui.Sayf("Configuring boot options: %d second delay, EFI Secure Boot enabled", s.BootDelay)
	} else if bootDelayMs > 0 {
		ui.Sayf("Configuring boot options: %d second delay", s.BootDelay)
	} else if efiSecureBoot {
		ui.Say("Configuring boot options: EFI Secure Boot enabled")
	}

	if err := vm.SetBootOptions(bootDelayMs, efiSecureBoot); err != nil {
		state.Put("error", fmt.Errorf("error configuring boot options: %w", err))
		return multistep.ActionHalt
	}

	ui.Say("Boot options configured successfully")
	return multistep.ActionContinue
}

func (s *StepConfigureBootOptions) Cleanup(state multistep.StateBag) {
	// No cleanup needed
}
