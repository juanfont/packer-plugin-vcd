package common

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"time"

	"github.com/hashicorp/packer-plugin-sdk/communicator"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/juanfont/packer-plugin-vcd/builder/vcd/driver"
)

type ShutdownConfig struct {
	// Specify a virtual machine guest shutdown command. This command will be run using
	// the `communicator`. Otherwise, the VMware Tools are used to gracefully shut down
	// the virtual machine.
	Command string `mapstructure:"shutdown_command"`
	// Amount of time to wait for graceful shut down of the virtual machine.
	// Defaults to `5m` (5 minutes).
	// This will likely need to be modified if the `communicator` is 'none'.
	Timeout time.Duration `mapstructure:"shutdown_timeout"`
	// Packer normally halts the virtual machine after all provisioners have
	// run when no `shutdown_command` is defined. If this is set to `true`, Packer
	// *will not* halt the virtual machine but will assume that you will send the stop
	// signal yourself through a `preseed.cfg`, a script or the final provisioner.
	// Packer will wait for a default of 5 minutes until the virtual machine is shutdown.
	// The timeout can be changed using `shutdown_timeout` option.
	DisableShutdown bool `mapstructure:"disable_shutdown"`
}

func (c *ShutdownConfig) Prepare(comm communicator.Config) (warnings []string, errs []error) {
	if c.Timeout == 0 {
		c.Timeout = 5 * time.Minute
	}

	if comm.Type == "none" && c.Command != "" {
		warnings = append(warnings, "The parameter `shutdown_command` is ignored as it requires a `communicator`.")
	}

	return
}

type StepShutdown struct {
	Config         *ShutdownConfig
	CommType       string // "none", "ssh", "winrm", etc.
}

func (s *StepShutdown) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	ui := state.Get("ui").(packersdk.Ui)
	vm := state.Get("vm").(driver.VirtualMachine)

	if off, _ := vm.IsPoweredOff(); off {
		ui.Say("Virtual machine is already powered off.")
		return multistep.ActionContinue
	}

	// Check if we have a communicator (not "none")
	hasCommunicator := s.CommType != "" && s.CommType != "none"

	if !hasCommunicator {
		// No communicator - just wait for VM to shutdown on its own
		ui.Sayf("Please shutdown virtual machine within %s.", s.Config.Timeout)
	} else if s.Config.DisableShutdown {
		ui.Say("Automatic shutdown disabled. Please shutdown virtual machine.")
	} else if s.Config.Command != "" {
		// Run shutdown command via communicator
		comm, _ := state.Get("communicator").(packersdk.Communicator)
		ui.Say("Running shutdown command...")
		log.Printf("[INFO] Shutdown command: %s", s.Config.Command)
		var stdout, stderr bytes.Buffer
		cmd := &packersdk.RemoteCmd{
			Command: s.Config.Command,
			Stdout:  &stdout,
			Stderr:  &stderr,
		}
		err := comm.Start(ctx, cmd)
		if err != nil {
			state.Put("error", fmt.Errorf("error sending shutdown command: %s", err))
			return multistep.ActionHalt
		}
	} else {
		// No shutdown command specified - try VMware Tools graceful shutdown
		ui.Sayf("Shutting down virtual machine via VMware Tools (timeout: %s)...", s.Config.Timeout)
		err := vm.Shutdown()
		if err != nil {
			state.Put("error", fmt.Errorf("error shutting down virtual machine: %v", err))
			return multistep.ActionHalt
		}
	}

	log.Printf("[INFO] Waiting a maximum of %s for shutdown to complete.", s.Config.Timeout)
	err := vm.WaitForPowerOff(ctx, s.Config.Timeout)
	if err != nil {
		state.Put("error", err)
		return multistep.ActionHalt
	}

	return multistep.ActionContinue
}

func (s *StepShutdown) Cleanup(state multistep.StateBag) {}
