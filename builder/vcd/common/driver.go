package common

import "github.com/vmware/go-vcloud-director/v3/govcd"

type Driver interface {
	NewVM() (*govcd.VM, error)
}

type ConnectConfig struct {
	Host              string
	Username          string
	Password          string
	Token             string
	Insecure          bool
	VirtualDatacenter string
}
