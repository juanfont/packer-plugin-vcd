package common

import (
	"fmt"
)

type LocationConfig struct {
	// The name of the virtual machine.
	VMName string `mapstructure:"vm_name"`
	// The  vApp where the virtual machine is created.
	VApp string `mapstructure:"vapp"`
	// The VDC where the virtual machine is created.
	VDC string `mapstructure:"vdc"`
}

func (c *LocationConfig) Prepare() []error {
	var errs []error

	if c.VMName == "" {
		errs = append(errs, fmt.Errorf("'vm_name' is required"))
	}
	if c.VApp == "" && c.VDC == "" {
		errs = append(errs, fmt.Errorf("'vapp' or 'vdc' is required"))
	}

	return errs
}
