package common

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/juanfont/packer-plugin-vcd/builder/vcd/driver"
)

//go:generate packer-sdc struct-markdown
//go:generate packer-sdc mapstructure-to-hcl2 -type WaitIpConfig

// WaitIpConfig contains configuration for waiting for VM IP address
type WaitIpConfig struct {
	// Time to wait for the VM to get an IP address. Defaults to 30m.
	WaitTimeout time.Duration `mapstructure:"ip_wait_timeout"`

	// Time to wait after IP is discovered before considering it stable. Defaults to 5s.
	SettleTimeout time.Duration `mapstructure:"ip_settle_timeout"`
}

func (c *WaitIpConfig) Prepare() []error {
	var errs []error

	if c.WaitTimeout == 0 {
		c.WaitTimeout = 30 * time.Minute
	}
	if c.SettleTimeout == 0 {
		c.SettleTimeout = 5 * time.Second
	}

	return errs
}

// StepWaitForIP waits for the VM to acquire an IP address.
// This is needed before the communicator can connect to the VM.
type StepWaitForIP struct {
	Config *WaitIpConfig
}

func (s *StepWaitForIP) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	ui := state.Get("ui").(packersdk.Ui)
	vm := state.Get("vm").(driver.VirtualMachine)

	timeout := s.Config.WaitTimeout
	settleTimeout := s.Config.SettleTimeout

	ui.Sayf("Waiting for VM to acquire IP address (timeout: %s)...", timeout)

	deadline := time.Now().Add(timeout)
	var lastIP string
	var settleStart time.Time

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return multistep.ActionHalt
		case <-ticker.C:
			if time.Now().After(deadline) {
				err := fmt.Errorf("timeout waiting for VM IP address after %s", timeout)
				state.Put("error", err)
				ui.Error(err.Error())
				return multistep.ActionHalt
			}

			ip, err := vm.GetIPAddress()
			if err != nil {
				// Log but continue polling
				ui.Sayf("Warning: error getting IP: %v", err)
				continue
			}

			if ip == "" {
				// No IP yet, reset settle timer
				lastIP = ""
				settleStart = time.Time{}
				continue
			}

			// Got an IP
			if ip != lastIP {
				// IP changed, restart settle timer
				ui.Sayf("Found IP address: %s (waiting for it to settle...)", ip)
				lastIP = ip
				settleStart = time.Now()
				continue
			}

			// IP is the same as before, check if it's been stable long enough
			if time.Since(settleStart) >= settleTimeout {
				ui.Sayf("IP address settled: %s", ip)
				state.Put("ip", ip)
				return multistep.ActionContinue
			}
		}
	}
}

func (s *StepWaitForIP) Cleanup(state multistep.StateBag) {
	// Nothing to clean up
}
