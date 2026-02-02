// Copyright 2025 Juan Font
// BSD-3-Clause

package common

import (
	"context"
	"fmt"
	"io"
	"net/netip"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
	"github.com/hashicorp/packer-plugin-sdk/multistep/commonsteps"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
)

// StepModifyISO modifies the downloaded ISO to include cd_content and cd_files
// This is needed because VCD only has one media slot, so we can't attach
// a separate CD for additional content.
type StepModifyISO struct {
	Config *commonsteps.CDConfig

	modifiedISOPath string
	debugFiles      []string
}

func (s *StepModifyISO) Run(_ context.Context, state multistep.StateBag) multistep.StepAction {
	// Check if there's anything to add
	if s.Config == nil {
		return multistep.ActionContinue
	}

	hasContent := len(s.Config.CDContent) > 0
	hasFiles := len(s.Config.CDFiles) > 0

	if !hasContent && !hasFiles {
		return multistep.ActionContinue
	}

	ui := state.Get("ui").(packersdk.Ui)

	// Get the downloaded ISO path
	isoPath, ok := state.Get("iso_path").(string)
	if !ok || isoPath == "" {
		state.Put("error", fmt.Errorf("iso_path not found in state"))
		return multistep.ActionHalt
	}

	ui.Say("Modifying ISO to include cd_content/cd_files...")

	// Create modifier
	modifier := NewISOModifier(isoPath)

	// Check if this is a UDF ISO (Windows) and verify tools are available
	isUDF, err := modifier.IsUDF()
	if err != nil {
		ui.Message(fmt.Sprintf("Warning: Could not detect filesystem type: %v", err))
	} else if isUDF {
		ui.Message("Detected UDF filesystem (Windows ISO)")
		if err := CheckUDFTools(); err != nil {
			state.Put("error", err)
			return multistep.ActionHalt
		}
	}

	// Build template variables from state
	templateVars := s.buildTemplateVars(state, ui)

	// Add cd_content entries (with template variable substitution)
	for path, content := range s.Config.CDContent {
		// Process template variables in content
		processedContent := s.processTemplateVars(content, templateVars)
		modifier.AddContent(path, []byte(processedContent))
		ui.Message(fmt.Sprintf("  Adding content: %s (%d bytes)", path, len(processedContent)))

		// Save processed content to /tmp for debugging
		debugPath := filepath.Join("/tmp", "packer-debug-"+filepath.Base(path))
		if err := os.WriteFile(debugPath, []byte(processedContent), 0644); err == nil {
			ui.Message(fmt.Sprintf("  Debug: saved processed content to %s", debugPath))
			s.debugFiles = append(s.debugFiles, debugPath)
		}
	}

	// Add cd_files entries
	for _, localPath := range s.Config.CDFiles {
		fi, err := os.Stat(localPath)
		if err != nil {
			state.Put("error", fmt.Errorf("failed to stat cd_file %s: %w", localPath, err))
			return multistep.ActionHalt
		}

		if fi.IsDir() {
			// Walk directory and add all files
			baseName := filepath.Base(localPath)
			err := filepath.Walk(localPath, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if info.IsDir() {
					return nil
				}

				relPath, err := filepath.Rel(localPath, path)
				if err != nil {
					return err
				}

				isoPath := filepath.Join(baseName, relPath)
				ui.Message(fmt.Sprintf("  Adding file: %s", isoPath))
				return modifier.AddFile(isoPath, path)
			})
			if err != nil {
				state.Put("error", fmt.Errorf("failed to add directory %s: %w", localPath, err))
				return multistep.ActionHalt
			}
		} else {
			// Single file - add to root
			isoPath := filepath.Base(localPath)

			// Check if cd_content already has this path (cd_content takes precedence)
			if _, exists := s.Config.CDContent[isoPath]; exists {
				ui.Message(fmt.Sprintf("  Skipping %s (overridden by cd_content)", isoPath))
				continue
			}

			ui.Message(fmt.Sprintf("  Adding file: %s", isoPath))
			if err := modifier.AddFile(isoPath, localPath); err != nil {
				state.Put("error", fmt.Errorf("failed to add file %s: %w", localPath, err))
				return multistep.ActionHalt
			}
		}
	}

	// Detect boot configuration
	bootConfig, err := modifier.DetectBootConfig()
	if err != nil {
		ui.Error(fmt.Sprintf("Warning: Could not detect boot configuration: %v", err))
		// Continue anyway - ISO will be non-bootable
	} else {
		if bootConfig.HasBIOSBoot {
			ui.Message(fmt.Sprintf("  Detected BIOS boot: %s", bootConfig.BIOSBootImage))
			ui.Message(fmt.Sprintf("  NeedsBootInfoTbl: %v", bootConfig.NeedsBootInfoTbl))
			if bootConfig.NeedsBootInfoTbl {
				ui.Message("  Boot-info-table patching: enabled (isolinux)")
			}
		}
		if bootConfig.HasUEFIBoot {
			ui.Message(fmt.Sprintf("  Detected UEFI boot: %s", bootConfig.UEFIBootImage))
		}
		if bootConfig.VolumeID != "" {
			ui.Message(fmt.Sprintf("  Volume ID: %s", bootConfig.VolumeID))
		}
		if !bootConfig.HasBIOSBoot && !bootConfig.HasUEFIBoot {
			ui.Say("Warning: No boot configuration detected. Modified ISO may not be bootable.")
		}
	}

	// Create modified ISO in temp directory
	tmpDir := os.TempDir()
	originalName := filepath.Base(isoPath)
	modifiedName := strings.TrimSuffix(originalName, filepath.Ext(originalName)) + "-modified.iso"
	modifiedPath := filepath.Join(tmpDir, modifiedName)

	ui.Say(fmt.Sprintf("Creating modified ISO: %s", modifiedName))

	checksum, err := modifier.CreateModifiedISO(modifiedPath)
	if err != nil {
		state.Put("error", fmt.Errorf("failed to create modified ISO: %w", err))
		return multistep.ActionHalt
	}

	s.modifiedISOPath = modifiedPath

	// Get modified ISO size
	modifiedInfo, _ := os.Stat(modifiedPath)
	originalInfo, _ := os.Stat(isoPath)

	ui.Say(fmt.Sprintf("Modified ISO created successfully"))
	ui.Message(fmt.Sprintf("  Original size: %d MB", originalInfo.Size()/(1024*1024)))
	ui.Message(fmt.Sprintf("  Modified size: %d MB", modifiedInfo.Size()/(1024*1024)))
	ui.Message(fmt.Sprintf("  SHA256: %s", checksum))

	// Update state with new ISO path
	state.Put("iso_path", modifiedPath)
	state.Put("iso_checksum", "sha256:"+checksum)
	state.Put("iso_modified", true)

	return multistep.ActionContinue
}

func (s *StepModifyISO) Cleanup(state multistep.StateBag) {
	ui := state.Get("ui").(packersdk.Ui)

	// Clean up the modified ISO
	if s.modifiedISOPath != "" {
		ui.Message(fmt.Sprintf("Cleaning up modified ISO: %s", s.modifiedISOPath))
		if err := os.Remove(s.modifiedISOPath); err != nil && !os.IsNotExist(err) {
			ui.Error(fmt.Sprintf("Warning: failed to remove modified ISO: %v", err))
		}
	}

	// Clean up debug files
	for _, debugFile := range s.debugFiles {
		if err := os.Remove(debugFile); err != nil && !os.IsNotExist(err) {
			ui.Error(fmt.Sprintf("Warning: failed to remove debug file %s: %v", debugFile, err))
		}
	}
}

// copyDir recursively copies a directory
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}

		return copyFile(path, dstPath)
	})
}

// copyFile copies a single file
func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}

// buildTemplateVars creates a map of template variables from state
// These can be used in cd_content with syntax like {{ .VMIP }}
func (s *StepModifyISO) buildTemplateVars(state multistep.StateBag, ui packersdk.Ui) map[string]string {
	vars := make(map[string]string)

	// VM IP address (from StepDiscoverIP)
	if vmIP, ok := state.Get("vm_ip").(string); ok && vmIP != "" {
		vars["VMIP"] = vmIP
		ui.Message(fmt.Sprintf("  Template variable: VMIP = %s", vmIP))
	}

	// Network gateway
	if gateway, ok := state.Get("network_gateway").(string); ok && gateway != "" {
		vars["VMGateway"] = gateway
		ui.Message(fmt.Sprintf("  Template variable: VMGateway = %s", gateway))
	}

	// Network netmask
	if netmask, ok := state.Get("network_netmask").(string); ok && netmask != "" {
		vars["VMNetmask"] = netmask
		ui.Message(fmt.Sprintf("  Template variable: VMNetmask = %s", netmask))

		// Also provide CIDR prefix (e.g., 24 for 255.255.255.0)
		if prefix := netmaskToPrefix(netmask); prefix != "" {
			vars["VMPrefix"] = prefix
			ui.Message(fmt.Sprintf("  Template variable: VMPrefix = %s", prefix))
		}
	}

	// DNS server
	if dns, ok := state.Get("network_dns").(string); ok && dns != "" {
		vars["VMDNS"] = dns
		ui.Message(fmt.Sprintf("  Template variable: VMDNS = %s", dns))
	}

	// HTTP IP (from StepHTTPIPDiscover)
	if httpIP, ok := state.Get("http_ip").(string); ok && httpIP != "" {
		vars["HTTPIP"] = httpIP
	}

	// HTTP Port
	if httpPort, ok := state.Get("http_port").(int); ok && httpPort > 0 {
		vars["HTTPPort"] = fmt.Sprintf("%d", httpPort)
	}

	return vars
}

// processTemplateVars replaces template variables in content
// Supports both {{ .VarName }} and {{.VarName}} syntax
func (s *StepModifyISO) processTemplateVars(content string, vars map[string]string) string {
	result := content

	for name, value := range vars {
		// Replace {{ .VarName }} (with spaces)
		result = strings.ReplaceAll(result, "{{ ."+name+" }}", value)
		// Replace {{.VarName}} (without spaces)
		result = strings.ReplaceAll(result, "{{."+name+"}}", value)
	}

	return result
}

// netmaskToPrefix converts a dotted-decimal netmask to CIDR prefix length
// e.g., "255.255.255.0" -> "24", "255.255.0.0" -> "16"
func netmaskToPrefix(netmask string) string {
	// Parse as IP address
	addr, err := netip.ParseAddr(netmask)
	if err != nil || !addr.Is4() {
		return ""
	}

	// Count the bits in the netmask
	bytes := addr.As4()
	var bits int
	for _, b := range bytes {
		for b > 0 {
			bits += int(b & 1)
			b >>= 1
		}
	}

	return fmt.Sprintf("%d", bits)
}
