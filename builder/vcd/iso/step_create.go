package iso

import "fmt"

type CreateConfig struct {
	// Specifies the virtual machine hardware version. Defaults to "vmx-19".
	// Refer to VMware documentation for supported hardware versions.
	Version string `mapstructure:"vm_version"`

	// The guest operating system identifier for the virtual machine.
	// Defaults to `other3xLinux64Guest`.
	GuestOSType string `mapstructure:"guest_os_type"`

	// Description for the virtual machine.
	Description string `mapstructure:"vm_description"`

	// The size of the primary disk in MB.
	// Defaults to 40960 (40 GB).
	DiskSizeMB int64 `mapstructure:"disk_size_mb"`
}

func (c *CreateConfig) Prepare() []error {
	var errs []error

	if c.Version == "" {
		c.Version = "vmx-19"
	}

	if c.GuestOSType == "" {
		c.GuestOSType = "other3xLinux64Guest"
	}

	if c.DiskSizeMB == 0 {
		c.DiskSizeMB = 40960 // 40 GB default
	}

	if c.DiskSizeMB < 1024 {
		errs = append(errs, fmt.Errorf("'disk_size_mb' must be at least 1024 (1 GB)"))
	}

	return errs
}
