package common

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/juanfont/packer-plugin-vcd/builder/vcd/driver"
	"github.com/vmware/go-vcloud-director/v3/govcd"
)

type StepResolveVApp struct {
	VDCName     string
	VAppName    string
	NetworkName string
	CreateVApp  bool
}

func (s *StepResolveVApp) Run(_ context.Context, state multistep.StateBag) multistep.StepAction {
	ui := state.Get("ui").(packersdk.Ui)
	d := state.Get("driver").(driver.Driver)

	// Check if VDC is already in state (from StepCreateTempCatalog)
	var vdc *govcd.Vdc
	if existingVdc, ok := state.GetOk("vdc"); ok {
		vdc = existingVdc.(*govcd.Vdc)
		ui.Sayf("Using VDC from state: %s", vdc.Vdc.Name)
	} else {
		// Get VDC
		ui.Sayf("Getting VDC: %s", s.VDCName)
		var err error
		vdc, err = d.GetVdc(s.VDCName)
		if err != nil {
			state.Put("error", fmt.Errorf("error getting VDC %s: %w", s.VDCName, err))
			return multistep.ActionHalt
		}
		state.Put("vdc", vdc)
	}

	// Try to get existing vApp
	if s.VAppName != "" {
		ui.Sayf("Looking for vApp: %s", s.VAppName)
		vapp, err := vdc.GetVAppByName(s.VAppName, true)
		if err == nil && vapp != nil {
			ui.Sayf("Found existing vApp: %s", s.VAppName)
			state.Put("vapp", vapp)
			state.Put("vapp_name", s.VAppName)
			state.Put("vapp_created", false)
			return multistep.ActionContinue
		}

		if !s.CreateVApp {
			state.Put("error", fmt.Errorf("vApp %s not found and create_vapp is false", s.VAppName))
			return multistep.ActionHalt
		}
	}

	// Create a new vApp
	vappName := s.VAppName
	if vappName == "" {
		vappName = fmt.Sprintf("packer-%d", time.Now().UnixNano())
	}

	ui.Sayf("Creating vApp: %s", vappName)
	vapp, err := d.CreateVApp(vdc, vappName, "Packer build vApp", s.NetworkName)
	if err != nil {
		state.Put("error", fmt.Errorf("error creating vApp: %w", err))
		return multistep.ActionHalt
	}

	state.Put("vapp", vapp)
	state.Put("vapp_name", vappName)
	state.Put("vapp_created", true)

	ui.Sayf("vApp created: %s", vappName)
	return multistep.ActionContinue
}

func (s *StepResolveVApp) Cleanup(state multistep.StateBag) {
	ui := state.Get("ui").(packersdk.Ui)

	vappCreated, ok := state.GetOk("vapp_created")
	if !ok || !vappCreated.(bool) {
		return
	}

	// Only clean up on failure
	_, cancelled := state.GetOk(multistep.StateCancelled)
	_, halted := state.GetOk(multistep.StateHalted)
	if !cancelled && !halted {
		return
	}

	vapp, ok := state.GetOk("vapp")
	if !ok {
		return
	}

	vappName, _ := state.GetOk("vapp_name")
	ui.Sayf("Deleting vApp: %s", vappName)

	task, err := vapp.(*govcd.VApp).Delete()
	if err != nil {
		ui.Errorf("Error deleting vApp: %s", err)
		return
	}
	if err := task.WaitTaskCompletion(); err != nil {
		ui.Errorf("Error waiting for vApp deletion: %s", err)
	}
}
