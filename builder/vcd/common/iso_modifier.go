// Copyright 2025 Juan Font
// BSD-3-Clause

package common

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/diskfs/go-diskfs"
	"github.com/diskfs/go-diskfs/filesystem"
	"github.com/diskfs/go-diskfs/filesystem/iso9660"
)

// ISOModifier handles reading and modifying ISO images
type ISOModifier struct {
	sourcePath string
	files      map[string][]byte // path -> content
}

// NewISOModifier creates a new ISO modifier for the given source ISO
func NewISOModifier(sourcePath string) *ISOModifier {
	return &ISOModifier{
		sourcePath: sourcePath,
		files:      make(map[string][]byte),
	}
}

// AddContent adds content to be included in the modified ISO
func (m *ISOModifier) AddContent(path string, content []byte) {
	// Normalize path - ISO paths typically use forward slashes
	path = filepath.ToSlash(path)
	// Ensure path doesn't start with /
	path = strings.TrimPrefix(path, "/")
	m.files[path] = content
}

// AddFile adds a file from disk to be included in the modified ISO
func (m *ISOModifier) AddFile(isoPath, localPath string) error {
	content, err := os.ReadFile(localPath)
	if err != nil {
		return fmt.Errorf("failed to read file %s: %w", localPath, err)
	}
	m.AddContent(isoPath, content)
	return nil
}

// BootConfig holds boot configuration detected from an ISO
type BootConfig struct {
	// BIOS boot
	HasBIOSBoot      bool
	BIOSBootImage    string // Path to boot image (e.g., boot/etfsboot.com)
	BIOSLoadSize     uint16 // Load size in sectors (usually 4 for no-emulation)
	NeedsBootInfoTbl bool   // True for isolinux/syslinux (needs boot info table patch)

	// UEFI boot
	HasUEFIBoot   bool
	UEFIBootImage string // Path to EFI boot image (e.g., efi/microsoft/boot/efisys.bin)

	// Volume info
	VolumeID string
}

// DetectBootConfig attempts to detect boot configuration from common ISO layouts
func (m *ISOModifier) DetectBootConfig() (*BootConfig, error) {
	d, err := diskfs.Open(m.sourcePath, diskfs.WithOpenMode(diskfs.ReadOnly))
	if err != nil {
		return nil, fmt.Errorf("failed to open ISO: %w", err)
	}
	defer d.Backend.Close()

	fs, err := d.GetFilesystem(0)
	if err != nil {
		return nil, fmt.Errorf("failed to get filesystem: %w", err)
	}

	config := &BootConfig{}

	// Try to get volume label from ISO9660
	if iso, ok := fs.(*iso9660.FileSystem); ok {
		config.VolumeID = iso.Label()
	}

	// Check for Windows boot files
	windowsBIOSPaths := []string{
		"/boot/etfsboot.com",
		"/BOOT/ETFSBOOT.COM",
	}
	windowsUEFIPaths := []string{
		"/efi/microsoft/boot/efisys.bin",
		"/EFI/MICROSOFT/BOOT/EFISYS.BIN",
		"/efi/microsoft/boot/efisys_noprompt.bin",
		"/EFI/MICROSOFT/BOOT/EFISYS_NOPROMPT.BIN",
	}

	// Check for Linux boot files
	linuxBIOSPaths := []string{
		"/isolinux/isolinux.bin",
		"/ISOLINUX/ISOLINUX.BIN",
		"/syslinux/syslinux.bin",
		"/boot/isolinux/isolinux.bin",
	}
	linuxUEFIPaths := []string{
		"/boot/grub/efi.img",
		"/boot/grub/x86_64-efi/grub.efi",
		"/EFI/BOOT/BOOTX64.EFI",
		"/efi/boot/bootx64.efi",
	}

	// Check Windows BIOS boot
	for _, path := range windowsBIOSPaths {
		if m.fileExists(fs, path) {
			config.HasBIOSBoot = true
			config.BIOSBootImage = strings.TrimPrefix(path, "/")
			config.BIOSLoadSize = 8 // Windows uses 8 sectors (4KB)
			break
		}
	}

	// Check Linux BIOS boot (isolinux/syslinux needs boot-info-table)
	if !config.HasBIOSBoot {
		for _, path := range linuxBIOSPaths {
			if m.fileExists(fs, path) {
				config.HasBIOSBoot = true
				config.BIOSBootImage = strings.TrimPrefix(path, "/")
				config.BIOSLoadSize = 4        // Standard isolinux
				config.NeedsBootInfoTbl = true // isolinux requires boot-info-table
				break
			}
		}
	}

	// Check Windows UEFI boot
	for _, path := range windowsUEFIPaths {
		if m.fileExists(fs, path) {
			config.HasUEFIBoot = true
			config.UEFIBootImage = strings.TrimPrefix(path, "/")
			break
		}
	}

	// Check Linux UEFI boot
	if !config.HasUEFIBoot {
		for _, path := range linuxUEFIPaths {
			if m.fileExists(fs, path) {
				config.HasUEFIBoot = true
				config.UEFIBootImage = strings.TrimPrefix(path, "/")
				break
			}
		}
	}

	return config, nil
}

func (m *ISOModifier) fileExists(fs filesystem.FileSystem, path string) bool {
	f, err := fs.OpenFile(path, os.O_RDONLY)
	if err != nil {
		return false
	}
	f.Close()
	return true
}

// CreateModifiedISO creates a new ISO with the added content
// Returns the SHA256 checksum of the new ISO
func (m *ISOModifier) CreateModifiedISO(outputPath string) (string, error) {
	// Check if this is a UDF ISO (Windows ISOs use UDF)
	isUDF, err := m.IsUDF()
	if err != nil {
		return "", fmt.Errorf("failed to detect filesystem type: %w", err)
	}

	if isUDF {
		return m.createModifiedUDFISO(outputPath)
	}

	return m.createModifiedISO9660(outputPath)
}

// createModifiedISO9660 creates a modified ISO for ISO9660 filesystems using xorriso.
// xorriso properly preserves all boot records (BIOS/UEFI El Torito, boot-info-table, etc.)
// which is critical for the modified ISO to remain bootable.
func (m *ISOModifier) createModifiedISO9660(outputPath string) (string, error) {
	// Check for xorriso
	xorrisoPath, err := exec.LookPath("xorriso")
	if err != nil {
		return "", fmt.Errorf("xorriso not found in PATH. Install it with: apt-get install xorriso")
	}

	// Create a temp directory for the files we want to add
	addDir, err := os.MkdirTemp("", "packer-iso-add-")
	if err != nil {
		return "", fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(addDir)

	// Write new content files to the temp directory
	for path, content := range m.files {
		fullPath := filepath.Join(addDir, path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			return "", fmt.Errorf("failed to create directory for %s: %w", path, err)
		}
		if err := os.WriteFile(fullPath, content, 0644); err != nil {
			return "", fmt.Errorf("failed to write file %s: %w", path, err)
		}
	}

	// Remove output file if it exists (xorriso needs a fresh file for -outdev)
	os.Remove(outputPath)

	// Build xorriso command:
	// -indev: read source ISO
	// -outdev: write complete new ISO (not multi-session append)
	// -boot_image any replay: preserve all boot records exactly
	// -map: add files
	args := []string{
		"-indev", m.sourcePath,
		"-outdev", outputPath,
		"-boot_image", "any", "replay",
	}

	// Add each file/directory to the ISO
	for path := range m.files {
		localPath := filepath.Join(addDir, path)
		isoPath := "/" + path
		args = append(args, "-map", localPath, isoPath)
	}

	cmd := exec.Command(xorrisoPath, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("xorriso failed: %w\nstderr: %s", err, stderr.String())
	}

	// Calculate checksum
	checksum, err := m.calculateChecksum(outputPath)
	if err != nil {
		return "", fmt.Errorf("failed to calculate checksum: %w", err)
	}

	return checksum, nil
}

func (m *ISOModifier) calculateChecksum(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// IsUDF checks if the ISO uses UDF filesystem (common for Windows ISOs)
func (m *ISOModifier) IsUDF() (bool, error) {
	return isUDFFilesystem(m.sourcePath)
}

// CheckUDFTools verifies that the required external tools for UDF ISO modification
// are available. Returns an error with installation instructions if tools are missing.
// Required tools: 7z (p7zip-full), mkisofs or genisoimage
func CheckUDFTools() error {
	var missing []string

	if _, err := exec.LookPath("7z"); err != nil {
		missing = append(missing, "7z (install: apt-get install p7zip-full)")
	}

	_, mkisofsErr := exec.LookPath("mkisofs")
	_, genisoimageErr := exec.LookPath("genisoimage")
	if mkisofsErr != nil && genisoimageErr != nil {
		missing = append(missing, "mkisofs or genisoimage (install: apt-get install genisoimage)")
	}

	if len(missing) > 0 {
		return fmt.Errorf("UDF ISO modification requires external tools that are not installed:\n  - %s", strings.Join(missing, "\n  - "))
	}

	return nil
}

// isUDFFilesystem checks if the ISO uses UDF filesystem (common for Windows ISOs)
func isUDFFilesystem(isoPath string) (bool, error) {
	f, err := os.Open(isoPath)
	if err != nil {
		return false, err
	}
	defer f.Close()

	// UDF Volume Recognition Sequence starts at sector 16 (32KB)
	// Look for "NSR02" or "NSR03" which indicate UDF
	// Also check for "BEA01" (Beginning Extended Area) which precedes UDF descriptors
	buf := make([]byte, 2048)

	// Check sectors 16-20 for UDF signatures
	for sector := 16; sector <= 20; sector++ {
		_, err := f.ReadAt(buf, int64(sector)*2048)
		if err != nil {
			continue
		}

		// Check for UDF identifiers at offset 1
		if len(buf) > 5 {
			id := string(buf[1:6])
			if id == "BEA01" || id == "NSR02" || id == "NSR03" || id == "TEA01" {
				return true, nil
			}
		}
	}

	return false, nil
}

// createModifiedUDFISO creates a modified ISO for Windows ISOs (UDF/ISO9660 dual format)
// Uses 7z to extract (handles UDF properly) and mkisofs/genisoimage to recreate with proper UDF support.
// This approach ensures files are visible in both ISO9660 and UDF filesystems.
func (m *ISOModifier) createModifiedUDFISO(outputPath string) (string, error) {
	// Check if 7z is available (for UDF extraction)
	sevenZipPath, err := exec.LookPath("7z")
	if err != nil {
		return "", fmt.Errorf("7z not found in PATH. Install it with: apt-get install p7zip-full (Debian/Ubuntu) or yum install p7zip (RHEL/CentOS)")
	}

	// Check if mkisofs or genisoimage is available (for recreation with UDF)
	mkisofsPath, err := exec.LookPath("mkisofs")
	if err != nil {
		mkisofsPath, err = exec.LookPath("genisoimage")
		if err != nil {
			return "", fmt.Errorf("mkisofs/genisoimage not found in PATH. Install with: apt-get install genisoimage")
		}
	}

	// Create temporary directory for extraction
	extractDir, err := os.MkdirTemp("", "packer-iso-extract-")
	if err != nil {
		return "", fmt.Errorf("failed to create extract directory: %w", err)
	}
	defer os.RemoveAll(extractDir)

	// Extract the source ISO using 7z (handles UDF filesystems properly)
	// 7z x = extract with full paths, -o = output directory
	extractArgs := []string{
		"x",
		m.sourcePath,
		"-o" + extractDir,
		"-y", // Yes to all prompts
	}

	cmd := exec.Command(sevenZipPath, extractArgs...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("7z extract failed: %w\nstderr: %s\nstdout: %s", err, stderr.String(), stdout.String())
	}

	// Verify extraction worked by checking for common Windows ISO files
	bootDir := filepath.Join(extractDir, "boot")
	if _, err := os.Stat(bootDir); os.IsNotExist(err) {
		// Try lowercase
		bootDir = filepath.Join(extractDir, "Boot")
		if _, err := os.Stat(bootDir); os.IsNotExist(err) {
			return "", fmt.Errorf("extraction appears to have failed: 'boot' directory not found in extracted ISO")
		}
	}

	// Add new files to the extracted directory
	for path, content := range m.files {
		fullPath := filepath.Join(extractDir, path)

		// Create parent directories
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			return "", fmt.Errorf("failed to create directory for %s: %w", path, err)
		}

		// Write file
		if err := os.WriteFile(fullPath, content, 0644); err != nil {
			return "", fmt.Errorf("failed to write file %s: %w", path, err)
		}
	}

	// Detect boot files from the extracted directory structure
	// Windows ISOs use specific paths for BIOS and UEFI boot images
	var biosBootImage, uefiBootImage string

	// Check for Windows BIOS boot image
	biosBootPaths := []string{
		"boot/etfsboot.com",
		"Boot/etfsboot.com",
		"BOOT/ETFSBOOT.COM",
	}
	for _, path := range biosBootPaths {
		fullPath := filepath.Join(extractDir, path)
		if _, err := os.Stat(fullPath); err == nil {
			biosBootImage = path
			break
		}
	}

	// Check for Windows UEFI boot image
	uefiBootPaths := []string{
		"efi/microsoft/boot/efisys.bin",
		"EFI/Microsoft/Boot/efisys.bin",
		"EFI/MICROSOFT/BOOT/EFISYS.BIN",
		"efi/microsoft/boot/efisys_noprompt.bin",
		"EFI/Microsoft/Boot/efisys_noprompt.bin",
	}
	for _, path := range uefiBootPaths {
		fullPath := filepath.Join(extractDir, path)
		if _, err := os.Stat(fullPath); err == nil {
			uefiBootImage = path
			break
		}
	}

	// Build mkisofs command to recreate ISO with UDF and boot support
	// Use flags compatible with Windows ISOs
	mkisofsArgs := []string{
		"-o", outputPath,
		"-V", m.getVolumeID(),
		"-iso-level", "3",       // Allow files > 4GB
		"-J",                    // Joliet extensions for Windows
		"-joliet-long",          // Allow long Joliet names
		"-udf",                  // Generate UDF filesystem
		"-allow-limited-size",   // Support large files in ISO9660/UDF
		"-r",                    // Rock Ridge for Unix compatibility
	}

	// Add BIOS boot if found
	if biosBootImage != "" {
		mkisofsArgs = append(mkisofsArgs,
			"-b", biosBootImage,
			"-no-emul-boot",
			"-boot-load-seg", "0x07C0",
			"-boot-load-size", "8", // Windows uses 8 sectors
		)
	}

	// Add UEFI boot if found
	if uefiBootImage != "" {
		// Use eltorito-alt-boot for dual BIOS/UEFI
		if biosBootImage != "" {
			mkisofsArgs = append(mkisofsArgs, "-eltorito-alt-boot")
		}
		mkisofsArgs = append(mkisofsArgs,
			"-e", uefiBootImage,
			"-no-emul-boot",
		)
	}

	// Add boot catalog if we have any boot entries
	if biosBootImage != "" || uefiBootImage != "" {
		mkisofsArgs = append(mkisofsArgs, "-c", "boot.cat")
	}

	// Add source directory
	mkisofsArgs = append(mkisofsArgs, extractDir)

	// Run mkisofs
	cmd = exec.Command(mkisofsPath, mkisofsArgs...)
	stdout.Reset()
	stderr.Reset()
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("mkisofs failed: %w\nstderr: %s\nstdout: %s", err, stderr.String(), stdout.String())
	}

	// Calculate checksum
	checksum, err := m.calculateChecksum(outputPath)
	if err != nil {
		return "", fmt.Errorf("failed to calculate checksum: %w", err)
	}

	return checksum, nil
}

// getVolumeID extracts the volume ID from the source ISO
func (m *ISOModifier) getVolumeID() string {
	f, err := os.Open(m.sourcePath)
	if err != nil {
		return "DISK"
	}
	defer f.Close()

	// Read Primary Volume Descriptor (sector 16)
	pvd := make([]byte, 2048)
	_, err = f.ReadAt(pvd, 16*2048)
	if err != nil {
		return "DISK"
	}

	// Volume ID is at offset 40, 32 bytes long
	if pvd[0] == 1 && string(pvd[1:6]) == "CD001" {
		volID := strings.TrimSpace(string(pvd[40:72]))
		if volID != "" {
			return volID
		}
	}

	return "DISK"
}

