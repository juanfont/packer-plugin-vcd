package common

import (
	"fmt"
)

type LocationConfig struct {
	// The name of the virtual machine.
	VMName string `mapstructure:"vm_name"`
	// The vApp where the virtual machine is created.
	// If not specified and create_vapp is true, a new vApp will be created.
	VApp string `mapstructure:"vapp"`
	// The VDC where the virtual machine is created.
	VDC string `mapstructure:"vdc"`
	// If true, create a new vApp if the specified vApp does not exist.
	// Defaults to true.
	CreateVApp bool `mapstructure:"create_vapp"`
	// The network to attach to the virtual machine.
	Network string `mapstructure:"network"`
	// The IP allocation mode for the network connection.
	// Valid values are: POOL, DHCP, MANUAL, NONE.
	// Defaults to POOL.
	IPAllocationMode string `mapstructure:"ip_allocation_mode"`
	// The static IP address for the virtual machine.
	// Required when ip_allocation_mode is MANUAL, unless auto_discover_ip is true.
	VMIPAddress string `mapstructure:"vm_ip"`
	// Auto-discover an available IP from the network's IP pool.
	// Requires ip_allocation_mode = "MANUAL". When enabled, finds the first
	// available IP from the network's static IP pool and makes network info
	// available as template variables: {{ .VMIP }}, {{ .Gateway }}, {{ .Netmask }}, {{ .DNS }}.
	AutoDiscoverIP bool `mapstructure:"auto_discover_ip"`
	// Override the gateway address discovered from the network.
	// Only used when auto_discover_ip = true.
	VMGateway string `mapstructure:"vm_gateway"`
	// Override the DNS server discovered from the network.
	// Only used when auto_discover_ip = true.
	VMDNS string `mapstructure:"vm_dns"`
	// The storage profile to use for the virtual machine.
	// If not specified, the default storage profile for the VDC will be used.
	StorageProfile string `mapstructure:"storage_profile"`
}

func (c *LocationConfig) Prepare() []error {
	var errs []error

	if c.VMName == "" {
		errs = append(errs, fmt.Errorf("'vm_name' is required"))
	}
	if c.VDC == "" {
		errs = append(errs, fmt.Errorf("'vdc' is required"))
	}

	// Default to creating vApp if not specified
	if c.VApp == "" {
		c.CreateVApp = true
	}

	// Validate IP allocation mode
	if c.IPAllocationMode == "" {
		c.IPAllocationMode = "POOL"
	}
	validModes := map[string]bool{"POOL": true, "DHCP": true, "MANUAL": true, "NONE": true}
	if !validModes[c.IPAllocationMode] {
		errs = append(errs, fmt.Errorf("'ip_allocation_mode' must be one of: POOL, DHCP, MANUAL, NONE"))
	}

	// Validate auto_discover_ip requires MANUAL allocation mode
	if c.AutoDiscoverIP && c.IPAllocationMode != "MANUAL" {
		errs = append(errs, fmt.Errorf("'auto_discover_ip' requires 'ip_allocation_mode' to be MANUAL"))
	}

	// Validate vm_ip is set when using MANUAL allocation (unless auto_discover_ip is enabled)
	if c.IPAllocationMode == "MANUAL" && c.VMIPAddress == "" && !c.AutoDiscoverIP {
		errs = append(errs, fmt.Errorf("'vm_ip' is required when 'ip_allocation_mode' is MANUAL (or enable 'auto_discover_ip')"))
	}

	// Validate vm_gateway and vm_dns are only used with auto_discover_ip
	if !c.AutoDiscoverIP && (c.VMGateway != "" || c.VMDNS != "") {
		errs = append(errs, fmt.Errorf("'vm_gateway' and 'vm_dns' are only used when 'auto_discover_ip' is true"))
	}

	return errs
}
