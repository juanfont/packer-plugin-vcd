package common

import "github.com/vmware/go-vcloud-director/v3/govcd"

type Driver interface {
	NewVM() (*govcd.VM, error)
}
