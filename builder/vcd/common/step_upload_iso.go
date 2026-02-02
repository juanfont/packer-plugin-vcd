package common

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/juanfont/packer-plugin-vcd/builder/vcd/driver"
	"github.com/vmware/go-vcloud-director/v3/govcd"
)

type StepUploadISO struct {
	// CacheISO when true checks if ISO already exists in catalog before uploading.
	CacheISO bool
	// CacheOverwrite when true will delete and re-upload existing ISO.
	CacheOverwrite bool
}

func (s *StepUploadISO) Run(_ context.Context, state multistep.StateBag) multistep.StepAction {
	ui := state.Get("ui").(packersdk.Ui)
	d := state.Get("driver").(driver.Driver)
	catalog := state.Get("catalog").(*govcd.Catalog)
	catalogName := state.Get("catalog_name").(string)

	// Get the local ISO path from the download step
	isoPathRaw, ok := state.GetOk("iso_path")
	if !ok {
		state.Put("error", fmt.Errorf("iso_path not found in state - did the download step run?"))
		return multistep.ActionHalt
	}
	isoPath := isoPathRaw.(string)

	// Generate media name from ISO filename
	mediaName := filepath.Base(isoPath)

	// If the ISO was modified (cd_content), include checksum in name to avoid cache collisions
	// when cd_content changes between builds
	if isoModified, _ := state.GetOk("iso_modified"); isoModified != nil && isoModified.(bool) {
		if checksum, ok := state.GetOk("iso_checksum"); ok {
			// Extract short hash from "sha256:abc123..." format
			checksumStr := checksum.(string)
			if len(checksumStr) > 14 { // "sha256:" + at least 7 chars
				shortHash := checksumStr[7:15] // First 8 chars of hash
				ext := filepath.Ext(mediaName)
				base := mediaName[:len(mediaName)-len(ext)]
				mediaName = fmt.Sprintf("%s-%s%s", base, shortHash, ext)
			}
		}
	}
	ui.Sayf("Preparing to upload ISO: %s", mediaName)

	// Check if media already exists (cache check)
	if s.CacheISO {
		existingMedia, err := catalog.GetMediaByName(mediaName, false)
		if err == nil && existingMedia != nil {
			if s.CacheOverwrite {
				ui.Sayf("Overwriting existing ISO in catalog: %s", mediaName)
				task, err := existingMedia.Delete()
				if err != nil {
					state.Put("error", fmt.Errorf("error deleting existing media: %w", err))
					return multistep.ActionHalt
				}
				if err := task.WaitTaskCompletion(); err != nil {
					state.Put("error", fmt.Errorf("error waiting for media deletion: %w", err))
					return multistep.ActionHalt
				}
			} else {
				ui.Sayf("ISO already exists in catalog, skipping upload: %s", mediaName)
				state.Put("uploaded_media", existingMedia)
				state.Put("uploaded_media_name", mediaName)
				state.Put("media_was_uploaded", false) // Don't delete on cleanup
				return multistep.ActionContinue
			}
		}
	}

	// Upload the ISO
	ui.Sayf("Uploading ISO to catalog %s: %s", catalogName, mediaName)
	media, err := d.UploadMediaImage(catalog, mediaName, "Packer ISO upload", isoPath)
	if err != nil {
		state.Put("error", fmt.Errorf("error uploading ISO: %w", err))
		return multistep.ActionHalt
	}

	state.Put("uploaded_media", media)
	state.Put("uploaded_media_name", mediaName)
	state.Put("media_was_uploaded", true) // Mark for cleanup if using temp catalog

	ui.Sayf("ISO uploaded successfully: %s", mediaName)
	return multistep.ActionContinue
}

func (s *StepUploadISO) Cleanup(state multistep.StateBag) {
	ui := state.Get("ui").(packersdk.Ui)

	// Only clean up if we're using a temp catalog (temp catalog cleanup handles it)
	// or if explicitly requested and we actually uploaded
	tempCatalog, _ := state.GetOk("temp_catalog")
	if tempCatalog != nil && tempCatalog.(bool) {
		// Temp catalog will be deleted entirely, no need to clean up media
		return
	}

	// For persistent catalogs, we don't delete cached ISOs
	// This matches vsphere behavior - ISOs stay cached for future builds
	wasUploaded, ok := state.GetOk("media_was_uploaded")
	if !ok || !wasUploaded.(bool) {
		return
	}

	// If build was cancelled/halted and we want to clean up uploaded media
	// from persistent catalogs, we could do it here. For now, leave cached.
	_, cancelled := state.GetOk(multistep.StateCancelled)
	_, halted := state.GetOk(multistep.StateHalted)
	if cancelled || halted {
		mediaName, _ := state.GetOk("uploaded_media_name")
		ui.Sayf("Build cancelled/halted. Uploaded ISO remains in catalog: %s", mediaName)
	}
}
