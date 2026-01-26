package common

import (
	"context"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
)

// StepSetManualIP sets the manually configured IP address in state for use
// by boot_command and cd_content templates. Used for MANUAL allocation mode.
type StepSetManualIP struct {
	ManualIP        string // The IP address to use
	OverrideGateway string // Optional gateway
	OverrideDNS     string // Optional DNS
}

func (s *StepSetManualIP) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	ui := state.Get("ui").(packersdk.Ui)

	// If no manual IP, nothing to do (DHCP mode)
	if s.ManualIP == "" {
		return multistep.ActionContinue
	}

	state.Put("vm_ip", s.ManualIP)
	ui.Sayf("Using manually configured IP address: %s", s.ManualIP)

	if s.OverrideGateway != "" {
		state.Put("network_gateway", s.OverrideGateway)
		ui.Sayf("Using configured gateway: %s", s.OverrideGateway)
	}
	if s.OverrideDNS != "" {
		state.Put("network_dns", s.OverrideDNS)
		ui.Sayf("Using configured DNS: %s", s.OverrideDNS)
	}

	return multistep.ActionContinue
}

func (s *StepSetManualIP) Cleanup(state multistep.StateBag) {
	// Nothing to clean up
}
