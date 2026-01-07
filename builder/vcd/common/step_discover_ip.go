package common

import (
	"context"
	"fmt"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/juanfont/packer-plugin-vcd/builder/vcd/driver"
	"github.com/vmware/go-vcloud-director/v3/govcd"
)

// StepDiscoverIP discovers an available IP address from the network's IP pool.
// This step is used when auto_discover_ip is enabled to automatically find
// an available IP from the network's static IP pool.
type StepDiscoverIP struct {
	NetworkName     string
	AutoDiscover    bool
	ManualIP        string // If set, skip discovery and use this IP
	OverrideGateway string // Optional override for gateway
	OverrideDNS     string // Optional override for DNS
}

func (s *StepDiscoverIP) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	ui := state.Get("ui").(packersdk.Ui)

	// If manual IP is set, use it directly
	if s.ManualIP != "" {
		state.Put("vm_ip", s.ManualIP)
		ui.Sayf("Using manually configured IP address: %s", s.ManualIP)
		return multistep.ActionContinue
	}

	// If auto-discover is not enabled, skip
	if !s.AutoDiscover {
		return multistep.ActionContinue
	}

	// Validate we have required dependencies
	if s.NetworkName == "" {
		state.Put("error", fmt.Errorf("auto_discover_ip requires a network to be specified"))
		return multistep.ActionHalt
	}

	d := state.Get("driver").(driver.Driver)
	vdc := state.Get("vdc").(*govcd.Vdc)

	ui.Say("Discovering available IP from network pool...")

	networkInfo, err := d.FindAvailableIP(vdc, s.NetworkName)
	if err != nil {
		state.Put("error", fmt.Errorf("failed to discover available IP: %w", err))
		return multistep.ActionHalt
	}

	// Apply overrides if specified
	gateway := networkInfo.Gateway
	if s.OverrideGateway != "" {
		gateway = s.OverrideGateway
	}

	dns := networkInfo.DNS1
	if s.OverrideDNS != "" {
		dns = s.OverrideDNS
	}

	ui.Sayf("Discovered network info:")
	ui.Sayf("  IP Address: %s", networkInfo.AvailableIP)
	ui.Sayf("  Gateway: %s", gateway)
	ui.Sayf("  Netmask: %s", networkInfo.Netmask)
	ui.Sayf("  DNS: %s", dns)

	// Store in state for use by other steps
	state.Put("vm_ip", networkInfo.AvailableIP)
	state.Put("network_gateway", gateway)
	state.Put("network_netmask", networkInfo.Netmask)
	state.Put("network_dns", dns)

	return multistep.ActionContinue
}

func (s *StepDiscoverIP) Cleanup(state multistep.StateBag) {
	// Nothing to clean up
}
