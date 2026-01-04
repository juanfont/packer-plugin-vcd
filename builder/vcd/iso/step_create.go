package iso

type CreateConfig struct {
	// Specifies the virtual machine hardware version. Defaults to the most
	// current virtual machine hardware version supported by the ESXi host.
	// Refer to [KB 315655](https://knowledge.broadcom.com/external/article?articleNumber=315655)
	// for more information on supported virtual hardware versions.
	Version uint `mapstructure:"vm_version"`

	// The guest operating system identifier for the virtual machine.
	// Defaults to `otherGuest`.
	GuestOSType string `mapstructure:"guest_os_type"`
}

func (c *CreateConfig) Prepare() []error {
	var errs []error

	return errs
}
