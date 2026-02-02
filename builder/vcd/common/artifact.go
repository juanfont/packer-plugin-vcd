package common

import (
	"fmt"

	"github.com/juanfont/packer-plugin-vcd/builder/vcd/driver"
)

const BuilderId = "vcd"

type Artifact struct {
	Name      string
	Location  LocationConfig
	VM        driver.VirtualMachine
	StateData map[string]interface{}
	Outconfig *string
}

func (a *Artifact) BuilderId() string {
	return BuilderId
}

func (a *Artifact) Files() []string {
	if a.Outconfig != nil {
		return []string{*a.Outconfig}
	}
	return nil
}

func (a *Artifact) Id() string {
	return fmt.Sprintf("%s/%s/%s", a.Location.VDC, a.Location.VApp, a.Name)
}

func (a *Artifact) String() string {
	return fmt.Sprintf("VCD VM: %s in vApp %s (VDC: %s)", a.Name, a.Location.VApp, a.Location.VDC)
}

func (a *Artifact) State(name string) interface{} {
	if a.StateData != nil {
		return a.StateData[name]
	}
	return nil
}

func (a *Artifact) Destroy() error {
	if a.VM == nil {
		return nil
	}

	// Power off if needed
	if on, _ := a.VM.IsPoweredOn(); on {
		if err := a.VM.PowerOff(); err != nil {
			return fmt.Errorf("error powering off VM: %w", err)
		}
	}

	// Delete the VM
	govcdVM := a.VM.GetVM()
	err := govcdVM.Delete()
	if err != nil {
		return fmt.Errorf("error deleting VM: %w", err)
	}
	return nil
}
