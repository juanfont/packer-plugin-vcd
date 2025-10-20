package driver

import "github.com/vmware/go-vcloud-director/v3/govcd"

type VirtualMachine interface {
	PowerOff() error
}

type VirtualMachineDriver struct {
	vm     *govcd.VM
	driver *VCDDriver
}
