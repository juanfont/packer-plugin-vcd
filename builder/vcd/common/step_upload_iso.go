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

	isoModified := false
	if v, ok := state.GetOk("iso_modified"); ok {
		isoModified, _ = v.(bool)
	}
	tempCatalog := false
	if v, ok := state.GetOk("temp_catalog"); ok {
		tempCatalog, _ = v.(bool)
	}

	// Check if media already exists (cache check)
	if shouldUseISOCache(s.CacheISO, isoModified, tempCatalog) {
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

// shouldUseISOCache reports whether an ISO already present in the catalog may
// be reused instead of uploaded.
//
// A modified ISO (cd_content/cd_files) usually embeds per-build values such as
// the VM's IP, so it is unique to this build: never look it up, and never leave
// the decision to the caller. A temporary catalog is empty by construction.
func shouldUseISOCache(cacheISO, isoModified, tempCatalog bool) bool {
	return cacheISO && !isoModified && !tempCatalog
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

	wasUploaded, ok := state.GetOk("media_was_uploaded")
	if !ok || !wasUploaded.(bool) {
		return
	}

	// A modified ISO is unique to this build and can never be a cache hit, so
	// leaving it in a persistent catalog only leaks storage (several GB per
	// build). StepMountISO's cleanup has already ejected it.
	if v, ok := state.GetOk("iso_modified"); ok {
		if modified, _ := v.(bool); modified {
			s.deleteUploadedMedia(state, ui)
			return
		}
	}

	// For persistent catalogs, we don't delete cached ISOs
	// This matches vsphere behavior - ISOs stay cached for future builds

	// If build was cancelled/halted and we want to clean up uploaded media
	// from persistent catalogs, we could do it here. For now, leave cached.
	_, cancelled := state.GetOk(multistep.StateCancelled)
	_, halted := state.GetOk(multistep.StateHalted)
	if cancelled || halted {
		mediaName, _ := state.GetOk("uploaded_media_name")
		ui.Sayf("Build cancelled/halted. Uploaded ISO remains in catalog: %s", mediaName)
	}
}

// deleteUploadedMedia removes the ISO this build uploaded. Failures are
// reported, not fatal: the build result must not depend on cleanup.
func (s *StepUploadISO) deleteUploadedMedia(state multistep.StateBag, ui packersdk.Ui) {
	mediaRaw, ok := state.GetOk("uploaded_media")
	if !ok {
		return
	}
	media, ok := mediaRaw.(*govcd.Media)
	if !ok || media == nil {
		return
	}
	name, _ := state.GetOk("uploaded_media_name")
	ui.Sayf("Deleting build-specific ISO from catalog: %v", name)
	task, err := media.Delete()
	if err != nil {
		ui.Errorf("Could not delete ISO %v (remove it manually): %s", name, err)
		return
	}
	if err := task.WaitTaskCompletion(); err != nil {
		ui.Errorf("Could not delete ISO %v (remove it manually): %s", name, err)
	}
}
