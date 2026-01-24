package common

import (
	"context"
	"fmt"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/juanfont/packer-plugin-vcd/builder/vcd/driver"
)

// StepQueryVMIP queries the IP address assigned to a VM by VCD.
// This is used when POOL allocation mode is used - VCD assigns the IP
// and we need to query it after VM creation for use in cd_content templates.
type StepQueryVMIP struct {
	OverrideGateway string
	OverrideDNS     string
	VDCName         string
	NetworkName     string
}

func (s *StepQueryVMIP) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	ui := state.Get("ui").(packersdk.Ui)

	// Check if we already have an IP in state (from manual config or previous discovery)
	if existingIP, ok := state.Get("vm_ip").(string); ok && existingIP != "" {
		ui.Sayf("Using existing IP from state: %s", existingIP)
		return multistep.ActionContinue
	}

	// Get the VM
	vmRaw := state.Get("vm")
	if vmRaw == nil {
		state.Put("error", fmt.Errorf("vm not found in state - VM must be created first"))
		return multistep.ActionHalt
	}
	vm := vmRaw.(driver.VirtualMachine)

	ui.Say("Querying VM IP address assigned by VCD...")

	// Get the IP from the VM's network configuration
	ip, err := vm.GetIPAddress()
	if err != nil {
		state.Put("error", fmt.Errorf("failed to get VM IP address: %w", err))
		return multistep.ActionHalt
	}

	if ip == "" {
		state.Put("error", fmt.Errorf("VM has no IP address assigned - check network configuration"))
		return multistep.ActionHalt
	}

	ui.Sayf("VM assigned IP address: %s", ip)
	state.Put("vm_ip", ip)

	// Get network info for gateway/netmask/DNS
	dRaw := state.Get("driver")
	if dRaw != nil && s.VDCName != "" && s.NetworkName != "" {
		d := dRaw.(driver.Driver)
		vdc, err := d.GetVdc(s.VDCName)
		if err == nil {
			networkInfo, err := d.GetNetworkInfo(vdc, s.NetworkName)
			if err == nil {
				gateway := networkInfo.Gateway
				if s.OverrideGateway != "" {
					gateway = s.OverrideGateway
				}
				dns := networkInfo.DNS1
				if s.OverrideDNS != "" {
					dns = s.OverrideDNS
				}

				state.Put("network_gateway", gateway)
				state.Put("network_netmask", networkInfo.Netmask)
				state.Put("network_dns", dns)

				ui.Sayf("Network info: Gateway=%s, Netmask=%s, DNS=%s", gateway, networkInfo.Netmask, dns)
			}
		}
	} else {
		// Use overrides if available
		if s.OverrideGateway != "" {
			state.Put("network_gateway", s.OverrideGateway)
		}
		if s.OverrideDNS != "" {
			state.Put("network_dns", s.OverrideDNS)
		}
	}

	return multistep.ActionContinue
}

func (s *StepQueryVMIP) Cleanup(state multistep.StateBag) {
	// Nothing to clean up
}
