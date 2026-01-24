package common

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/packer-plugin-sdk/bootcommand"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/template/interpolate"
	"github.com/juanfont/packer-plugin-vcd/builder/vcd/driver"
	"github.com/vmware/go-vcloud-director/v3/govcd"
)

//go:generate packer-sdc struct-markdown
//go:generate packer-sdc mapstructure-to-hcl2 -type BootCommandConfig

// BootCommandConfig contains configuration for sending boot commands to the VM.
type BootCommandConfig struct {
	bootcommand.BootConfig `mapstructure:",squash"`

	// Time in ms to wait between each key press. Defaults to 100ms.
	BootKeyInterval time.Duration `mapstructure:"boot_key_interval"`
}

func (c *BootCommandConfig) Prepare(ctx *interpolate.Context) []error {
	// Save the original BootWait to check if user explicitly set it to 0
	originalBootWait := c.BootWait

	errs := c.BootConfig.Prepare(ctx)

	// The SDK sets a 10s default for BootWait. If user explicitly set "0s",
	// restore it to 0 so we don't wait.
	if originalBootWait == 0 {
		c.BootWait = 0
	}

	return errs
}

// StepBootCommand runs the boot command via WMKS console
type StepBootCommand struct {
	Config *BootCommandConfig
	VMName string
	Ctx    interpolate.Context
}

type bootCommandTemplateData struct {
	HTTPIP   string
	HTTPPort int
	Name     string
	// Network info (populated when auto_discover_ip is enabled)
	VMIP    string
	Gateway string
	Netmask string
	DNS     string
}

func (s *StepBootCommand) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	ui := state.Get("ui").(packersdk.Ui)
	d := state.Get("driver").(driver.Driver)
	vm := state.Get("vm").(driver.VirtualMachine)

	if len(s.Config.BootCommand) == 0 {
		ui.Say("No boot command configured, skipping...")
		return multistep.ActionContinue
	}

	// Wait for boot
	if s.Config.BootWait > 0 {
		ui.Sayf("Waiting %s for VM to boot...", s.Config.BootWait)
		select {
		case <-time.After(s.Config.BootWait):
		case <-ctx.Done():
			return multistep.ActionHalt
		}
	}

	ui.Say("Connecting to VM console via WMKS...")

	// Get the underlying govcd VM to acquire MKS ticket
	govcdVM := vm.GetVM()
	client := d.GetClient()

	// Acquire MKS ticket with retry logic
	// The VM console may not be immediately ready after power on
	var ticket *driver.MksTicket
	maxRetries := 10
	retryDelay := 5 * time.Second
	var lastErr error
	for i := 0; i < maxRetries; i++ {
		ticket, lastErr = driver.AcquireMksTicket(client, govcdVM)
		if lastErr == nil {
			break
		}
		// Try direct method if link traversal fails
		ticket, lastErr = driver.AcquireMksTicketDirect(client, govcdVM.VM.HREF)
		if lastErr == nil {
			break
		}
		if i < maxRetries-1 {
			ui.Sayf("Waiting for VM console to be ready (attempt %d/%d)...", i+1, maxRetries)
			select {
			case <-time.After(retryDelay):
			case <-ctx.Done():
				return multistep.ActionHalt
			}
		}
	}
	if lastErr != nil {
		state.Put("error", fmt.Errorf("failed to acquire MKS ticket after %d retries: %w", maxRetries, lastErr))
		return multistep.ActionHalt
	}

	ui.Sayf("MKS ticket acquired (host: %s, port: %d)", ticket.Host, ticket.Port)

	// Connect to console
	insecure := true // TODO: get from config
	wmksClient := driver.NewWMKSClient(ticket, driver.WithInsecure(insecure))
	if err := wmksClient.Connect(); err != nil {
		state.Put("error", fmt.Errorf("failed to connect to WMKS console: %w", err))
		return multistep.ActionHalt
	}
	defer wmksClient.Close()

	ui.Say("Connected to VM console")

	// Prepare template data for boot command interpolation
	httpIP := ""
	httpPort := 0
	if ip, ok := state.GetOk("http_ip"); ok {
		httpIP = ip.(string)
	}
	if port, ok := state.GetOk("http_port"); ok {
		httpPort = port.(int)
	}

	// Get network info from state (populated by StepDiscoverIP)
	vmIP := ""
	gateway := ""
	netmask := ""
	dns := ""
	if ip, ok := state.GetOk("vm_ip"); ok {
		vmIP = ip.(string)
	}
	if gw, ok := state.GetOk("network_gateway"); ok {
		gateway = gw.(string)
	}
	if nm, ok := state.GetOk("network_netmask"); ok {
		netmask = nm.(string)
	}
	if d, ok := state.GetOk("network_dns"); ok {
		dns = d.(string)
	}

	s.Ctx.Data = &bootCommandTemplateData{
		HTTPIP:   httpIP,
		HTTPPort: httpPort,
		Name:     s.VMName,
		VMIP:     vmIP,
		Gateway:  gateway,
		Netmask:  netmask,
		DNS:      dns,
	}

	// Create boot command driver
	keyInterval := s.Config.BootKeyInterval
	if keyInterval == 0 {
		keyInterval = 100 * time.Millisecond
	}
	bootDriver := driver.NewWMKSBootDriver(wmksClient, keyInterval)

	// Parse and execute boot command
	ui.Say("Sending boot command...")

	// Interpolate the boot command to replace {{ .HTTPIP }}, {{ .HTTPPort }}, etc.
	flatBootCommand, err := interpolate.Render(s.Config.FlatBootCommand(), &s.Ctx)
	if err != nil {
		state.Put("error", fmt.Errorf("error interpolating boot command: %w", err))
		return multistep.ActionHalt
	}
	seq, err := bootcommand.GenerateExpressionSequence(flatBootCommand)
	if err != nil {
		state.Put("error", fmt.Errorf("error parsing boot command: %w", err))
		return multistep.ActionHalt
	}

	// Execute boot command with group interval
	groupInterval := s.Config.BootGroupInterval
	if groupInterval == 0 {
		groupInterval = 0 // No delay between groups by default
	}

	if err := seq.Do(ctx, bootDriver); err != nil {
		state.Put("error", fmt.Errorf("error running boot command: %w", err))
		return multistep.ActionHalt
	}

	ui.Say("Boot command completed successfully")

	return multistep.ActionContinue
}

func (s *StepBootCommand) Cleanup(state multistep.StateBag) {
	// Nothing to clean up
}

// BootCommandFromVM provides context for acquiring console access
type BootCommandFromVM interface {
	GetVM() *govcd.VM
}
