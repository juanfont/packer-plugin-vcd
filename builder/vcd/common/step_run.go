package common

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/juanfont/packer-plugin-vcd/builder/vcd/driver"
	"github.com/vmware/go-vcloud-director/v3/govcd"
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
	Config      *RunConfig
	SetOrder    bool
	VDCName     string
	NetworkName string
	MaxRetries  int // Max retries for IP conflicts (default 5)
}

const defaultMaxIPRetries = 5

func (s *StepRun) Run(_ context.Context, state multistep.StateBag) multistep.StepAction {
	ui := state.Get("ui").(packersdk.Ui)
	vm := state.Get("vm").(driver.VirtualMachine)

	maxRetries := s.MaxRetries
	if maxRetries == 0 {
		maxRetries = defaultMaxIPRetries
	}

	// Track IPs that have failed due to conflicts
	var failedIPs []string

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt == 0 {
			ui.Say("Powering on virtual machine...")
		} else {
			ui.Sayf("Retrying power on (attempt %d/%d)...", attempt+1, maxRetries+1)
		}

		err := vm.PowerOn()
		if err == nil {
			ui.Say("Virtual machine powered on.")
			return multistep.ActionContinue
		}

		// Check if this is an IP conflict error
		errStr := err.Error()
		isIPConflict := strings.Contains(errStr, "IP/MAC addresses have already been used") ||
			strings.Contains(errStr, "IP addresses:")

		if !isIPConflict {
			// Not an IP conflict, fail immediately
			state.Put("error", fmt.Errorf("error powering on VM: %w", err))
			ui.Error(err.Error())
			return multistep.ActionHalt
		}

		// Get the current IP that failed
		currentIP, _ := state.Get("vm_ip").(string)
		if currentIP != "" {
			failedIPs = append(failedIPs, currentIP)
		}

		// Check if we can retry (need driver, VDC, and network info)
		dRaw := state.Get("driver")
		vdcRaw := state.Get("vdc")
		if dRaw == nil || vdcRaw == nil || s.NetworkName == "" {
			// Can't retry without driver/VDC/network info
			err = fmt.Errorf("IP address %s is already in use. "+
				"Use 'ip_allocation_mode = \"POOL\"' to let VCD assign an available IP, "+
				"or specify a different 'vm_ip': %w", currentIP, err)
			state.Put("error", fmt.Errorf("error powering on VM: %w", err))
			ui.Error(err.Error())
			return multistep.ActionHalt
		}

		if attempt >= maxRetries {
			// Exhausted retries
			err = fmt.Errorf("IP address conflict after %d retries (tried IPs: %v): %w",
				maxRetries+1, failedIPs, err)
			state.Put("error", fmt.Errorf("error powering on VM: %w", err))
			ui.Error(err.Error())
			return multistep.ActionHalt
		}

		// Try to find a new IP
		d := dRaw.(driver.Driver)
		vdc := vdcRaw.(*govcd.Vdc)

		ui.Sayf("IP address %s is in use, trying to find another available IP...", currentIP)

		networkInfo, err := d.FindAvailableIPExcluding(vdc, s.NetworkName, failedIPs)
		if err != nil {
			state.Put("error", fmt.Errorf("failed to find alternative IP: %w", err))
			ui.Error(err.Error())
			return multistep.ActionHalt
		}

		newIP := networkInfo.AvailableIP
		ui.Sayf("Found new IP: %s, updating VM...", newIP)

		// Update the VM's IP address
		if err := vm.ChangeIPAddress(newIP); err != nil {
			state.Put("error", fmt.Errorf("failed to change VM IP to %s: %w", newIP, err))
			ui.Error(err.Error())
			return multistep.ActionHalt
		}

		// Update state with new IP
		state.Put("vm_ip", newIP)
		ui.Sayf("VM IP changed to %s", newIP)
	}

	// Should not reach here
	return multistep.ActionHalt
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

	// Check if VM is already powered off
	powered, err := vm.IsPoweredOn()
	if err != nil {
		ui.Errorf("Error checking VM power state: %s", err)
		return
	}
	if !powered {
		return
	}

	ui.Say("Powering off virtual machine...")

	err = vm.PowerOff()
	if err != nil {
		ui.Errorf("Error powering off VM: %s", err)
	}
}
