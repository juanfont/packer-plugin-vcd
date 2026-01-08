package common

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/juanfont/packer-plugin-vcd/builder/vcd/driver"
)

// StepUploadFloppy uploads a floppy image to the VCD catalog
type StepUploadFloppy struct {
	floppyMediaName string
}

func (s *StepUploadFloppy) Run(_ context.Context, state multistep.StateBag) multistep.StepAction {
	floppyPathRaw, ok := state.GetOk("floppy_path")
	if !ok {
		// No floppy to upload
		return multistep.ActionContinue
	}

	floppyPath := floppyPathRaw.(string)
	ui := state.Get("ui").(packersdk.Ui)
	d := state.Get("driver").(driver.Driver)
	catalogName := state.Get("catalog_name").(string)

	// Generate a unique name for the floppy media
	s.floppyMediaName = fmt.Sprintf("packer-floppy-%s-%d.flp",
		filepath.Base(floppyPath), time.Now().Unix())

	ui.Sayf("Uploading floppy image to catalog %s as %s...", catalogName, s.floppyMediaName)

	catalog, err := d.GetCatalog(catalogName)
	if err != nil {
		state.Put("error", fmt.Errorf("error getting catalog: %w", err))
		return multistep.ActionHalt
	}

	// Upload floppy image (checkFileIsIso=false to allow .flp files)
	uploadTask, err := catalog.UploadMediaFile(s.floppyMediaName, "Packer floppy image", floppyPath, 1024*1024, false)
	if err != nil {
		state.Put("error", fmt.Errorf("error uploading floppy image: %w", err))
		return multistep.ActionHalt
	}

	// Wait for upload to complete
	err = uploadTask.WaitTaskCompletion()
	if err != nil {
		state.Put("error", fmt.Errorf("error waiting for floppy upload: %w", err))
		return multistep.ActionHalt
	}

	ui.Say("Floppy image uploaded successfully")
	state.Put("floppy_media_name", s.floppyMediaName)
	state.Put("floppy_catalog_name", catalogName)

	return multistep.ActionContinue
}

func (s *StepUploadFloppy) Cleanup(state multistep.StateBag) {
	// Clean up the uploaded floppy media if build failed
	_, cancelled := state.GetOk(multistep.StateCancelled)
	_, halted := state.GetOk(multistep.StateHalted)
	if !cancelled && !halted {
		return
	}

	if s.floppyMediaName == "" {
		return
	}

	ui := state.Get("ui").(packersdk.Ui)
	d := state.Get("driver").(driver.Driver)
	catalogName, ok := state.GetOk("floppy_catalog_name")
	if !ok {
		return
	}

	ui.Sayf("Cleaning up floppy media: %s", s.floppyMediaName)

	catalog, err := d.GetCatalog(catalogName.(string))
	if err != nil {
		ui.Errorf("Error getting catalog for cleanup: %s", err)
		return
	}

	media, err := catalog.GetMediaByName(s.floppyMediaName, false)
	if err != nil {
		return // Media might not exist
	}

	task, err := media.Delete()
	if err != nil {
		ui.Errorf("Error deleting floppy media: %s", err)
		return
	}

	task.WaitTaskCompletion()
}
