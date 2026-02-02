package common

import (
	"context"
	"fmt"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/juanfont/packer-plugin-vcd/builder/vcd/driver"
)

type StepMountISO struct{}

func (s *StepMountISO) Run(_ context.Context, state multistep.StateBag) multistep.StepAction {
	ui := state.Get("ui").(packersdk.Ui)
	vm := state.Get("vm").(driver.VirtualMachine)
	catalogName := state.Get("catalog_name").(string)
	mediaName := state.Get("uploaded_media_name").(string)

	ui.Sayf("Mounting ISO: %s from catalog %s", mediaName, catalogName)

	err := vm.InsertMedia(catalogName, mediaName)
	if err != nil {
		state.Put("error", fmt.Errorf("error mounting ISO: %w", err))
		return multistep.ActionHalt
	}

	state.Put("iso_mounted", true)
	ui.Say("ISO mounted successfully")
	return multistep.ActionContinue
}

func (s *StepMountISO) Cleanup(state multistep.StateBag) {
	ui := state.Get("ui").(packersdk.Ui)

	isoMounted, ok := state.GetOk("iso_mounted")
	if !ok || !isoMounted.(bool) {
		return
	}

	vmRaw, ok := state.GetOk("vm")
	if !ok {
		return
	}
	vm := vmRaw.(driver.VirtualMachine)

	catalogName, ok := state.GetOk("catalog_name")
	if !ok {
		return
	}

	mediaName, ok := state.GetOk("uploaded_media_name")
	if !ok {
		return
	}

	ui.Sayf("Ejecting ISO: %s", mediaName)
	err := vm.EjectMedia(catalogName.(string), mediaName.(string))
	if err != nil {
		ui.Errorf("Error ejecting ISO: %s", err)
	}
}
