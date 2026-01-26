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
	// The VM hardware version. Defaults to vmx-21 (ESXi 8.0+).
	// Examples: vmx-19 (ESXi 7.0 U2+), vmx-20 (ESXi 8.0), vmx-21 (ESXi 8.0 U2+)
	HardwareVersion string `mapstructure:"hw_version"`
	// Force entry into the BIOS setup screen during boot. Defaults to `false`.
	ForceBIOSSetup bool `mapstructure:"force_bios_setup"`
	// Enable virtual trusted platform module (TPM) device for the virtual
	// machine. Defaults to `false`.
	VTPMEnabled bool `mapstructure:"vTPM"`
	// Boot delay in seconds. This adds a delay between power-on and boot,
	// giving time for the "Press any key to boot from CD" prompt to appear.
	// Useful for EFI boot with Windows ISOs. Defaults to 0 (no delay).
	BootDelay int `mapstructure:"boot_delay"`
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
