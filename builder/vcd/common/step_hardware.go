package common

import "fmt"

type HardwareConfig struct {
	// The number of virtual CPUs cores for the virtual machine.
	CPUs int32 `mapstructure:"CPUs"`
	// Cores per socket for the virtual machine.
	CoresPerSocket int32 `mapstructure:"cores_per_socket"`
	// Enable CPU hot plug setting for virtual machine. Defaults to `false`
	CpuHotAddEnabled bool `mapstructure:"CPU_hot_plug"`

	// The amount of memory for the virtual machine (MB)
	Memory int64 `mapstructure:"memory"`
	// Enable memory hot add setting for virtual machine. Defaults to `false`.
	MemoryHotAddEnabled bool `mapstructure:"RAM_hot_plug"`

	// Enable nested hardware virtualization for the virtual machine.
	NestedHV bool `mapstructure:"NestedHV"`
	// The firmware for the virtual machine.
	//
	// The available options for this setting are: 'bios', 'efi', and
	// 'efi-secure'.
	//
	// -> **Note:** Use `efi-secure` for UEFI Secure Boot.
	Firmware string `mapstructure:"firmware"`
	// Force entry into the BIOS setup screen during boot. Defaults to `false`.
	ForceBIOSSetup bool `mapstructure:"force_bios_setup"`
	// Enable virtual trusted platform module (TPM) device for the virtual
	// machine. Defaults to `false`.
	VTPMEnabled bool `mapstructure:"vTPM"`
}

func (c *HardwareConfig) Prepare() []error {
	var errs []error

	if c.Firmware != "" && c.Firmware != "bios" && c.Firmware != "efi" && c.Firmware != "efi-secure" {
		errs = append(errs, fmt.Errorf("'firmware' must be '', 'bios', 'efi' or 'efi-secure'"))
	}

	if c.VTPMEnabled && c.Firmware != "efi" && c.Firmware != "efi-secure" {
		errs = append(errs, fmt.Errorf("'vTPM' could be enabled only when 'firmware' set to 'efi' or 'efi-secure'"))
	}

	return errs
}
