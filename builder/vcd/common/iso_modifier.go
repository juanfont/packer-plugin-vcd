// Copyright 2025 Juan Font
// BSD-3-Clause

package common

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/diskfs/go-diskfs"
	"github.com/diskfs/go-diskfs/disk"
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
	isUDF, err := m.isUDFFilesystem()
	if err != nil {
		return "", fmt.Errorf("failed to detect filesystem type: %w", err)
	}

	if isUDF {
		return m.createModifiedUDFISO(outputPath)
	}

	return m.createModifiedISO9660(outputPath)
}

// createModifiedISO9660 creates a modified ISO for ISO9660 filesystems using go-diskfs
func (m *ISOModifier) createModifiedISO9660(outputPath string) (string, error) {
	// Detect boot configuration from source
	bootConfig, err := m.DetectBootConfig()
	if err != nil {
		return "", fmt.Errorf("failed to detect boot config: %w", err)
	}

	// Open source ISO to read files
	srcDisk, err := diskfs.Open(m.sourcePath, diskfs.WithOpenMode(diskfs.ReadOnly))
	if err != nil {
		return "", fmt.Errorf("failed to open source ISO: %w", err)
	}
	defer srcDisk.Backend.Close()

	srcFS, err := srcDisk.GetFilesystem(0)
	if err != nil {
		return "", fmt.Errorf("failed to get source filesystem: %w", err)
	}

	// Get source ISO size to estimate new size
	srcInfo, err := os.Stat(m.sourcePath)
	if err != nil {
		return "", fmt.Errorf("failed to stat source ISO: %w", err)
	}

	// Add extra space for new content
	extraSize := int64(0)
	for _, content := range m.files {
		extraSize += int64(len(content)) + 2048 // content + overhead
	}

	// Round up to nearest 2048-byte block and add padding
	newSize := srcInfo.Size() + extraSize + (100 * 1024 * 1024) // Add 100MB padding
	newSize = ((newSize + 2047) / 2048) * 2048

	// Remove output file if it exists
	os.Remove(outputPath)

	// Create new disk image with 2048 byte sectors (required for ISO9660)
	newDisk, err := diskfs.Create(outputPath, newSize, diskfs.SectorSize(2048))
	if err != nil {
		return "", fmt.Errorf("failed to create new ISO: %w", err)
	}
	// Note: we close newDisk.Backend manually after Finalize() so we can patch the ISO

	// Configure ISO filesystem
	isoSpec := disk.FilesystemSpec{
		Partition:   0,
		FSType:      filesystem.TypeISO9660,
		VolumeLabel: bootConfig.VolumeID,
	}

	newFS, err := newDisk.CreateFilesystem(isoSpec)
	if err != nil {
		return "", fmt.Errorf("failed to create ISO filesystem: %w", err)
	}

	isoFS, ok := newFS.(*iso9660.FileSystem)
	if !ok {
		return "", fmt.Errorf("created filesystem is not ISO9660")
	}

	// Copy all files from source ISO, collecting symlinks for later resolution
	symlinks := make(map[string]bool) // paths that are symlinks
	err = m.copyFilesRecursive(srcFS, isoFS, "/", symlinks)
	if err != nil {
		return "", fmt.Errorf("failed to copy files: %w", err)
	}

	// Resolve symlinks by detecting their targets
	if err := m.resolveSymlinks(srcFS, isoFS, symlinks); err != nil {
		return "", fmt.Errorf("failed to resolve symlinks: %w", err)
	}

	// Add new content files (overwriting if they exist)
	for path, content := range m.files {
		fullPath := "/" + path

		// Create parent directories
		dir := filepath.Dir(fullPath)
		if dir != "/" && dir != "." {
			if err := m.mkdirAll(isoFS, dir); err != nil {
				return "", fmt.Errorf("failed to create directory %s: %w", dir, err)
			}
		}

		// Write file
		f, err := isoFS.OpenFile(fullPath, os.O_CREATE|os.O_WRONLY)
		if err != nil {
			return "", fmt.Errorf("failed to create file %s: %w", fullPath, err)
		}

		_, err = f.Write(content)
		f.Close()
		if err != nil {
			return "", fmt.Errorf("failed to write file %s: %w", fullPath, err)
		}
	}

	// Configure finalize options
	finalizeOptions := iso9660.FinalizeOptions{
		RockRidge:        true,
		VolumeIdentifier: bootConfig.VolumeID,
	}

	if bootConfig.HasBIOSBoot && bootConfig.BIOSBootImage != "" {
		elTorito := &iso9660.ElTorito{
			BootCatalog: "boot.catalog",
			Entries: []*iso9660.ElToritoEntry{
				{
					Platform:  iso9660.BIOS,
					Emulation: iso9660.NoEmulation,
					BootFile:  bootConfig.BIOSBootImage,
					LoadSize:  bootConfig.BIOSLoadSize,
				},
			},
		}

		// Add UEFI boot entry if present
		if bootConfig.HasUEFIBoot && bootConfig.UEFIBootImage != "" {
			elTorito.Entries = append(elTorito.Entries, &iso9660.ElToritoEntry{
				Platform:  iso9660.EFI,
				Emulation: iso9660.NoEmulation,
				BootFile:  bootConfig.UEFIBootImage,
			})
		}

		finalizeOptions.ElTorito = elTorito
	} else if bootConfig.HasUEFIBoot && bootConfig.UEFIBootImage != "" {
		// UEFI only
		finalizeOptions.ElTorito = &iso9660.ElTorito{
			BootCatalog: "boot.catalog",
			Entries: []*iso9660.ElToritoEntry{
				{
					Platform:  iso9660.EFI,
					Emulation: iso9660.NoEmulation,
					BootFile:  bootConfig.UEFIBootImage,
				},
			},
		}
	}

	// Finalize the ISO
	if err := isoFS.Finalize(finalizeOptions); err != nil {
		return "", fmt.Errorf("failed to finalize ISO: %w", err)
	}

	// Close the disk so we can patch it
	newDisk.Backend.Close()

	// Patch boot-info-table if needed (for isolinux)
	if bootConfig.NeedsBootInfoTbl && bootConfig.BIOSBootImage != "" {
		if err := m.patchBootInfoTable(outputPath, bootConfig.BIOSBootImage); err != nil {
			return "", fmt.Errorf("failed to patch boot-info-table: %w", err)
		}
	}

	// Calculate checksum
	checksum, err := m.calculateChecksum(outputPath)
	if err != nil {
		return "", fmt.Errorf("failed to calculate checksum: %w", err)
	}

	return checksum, nil
}

func (m *ISOModifier) copyFilesRecursive(srcFS, dstFS filesystem.FileSystem, path string, symlinks map[string]bool) error {
	return m.copyFilesRecursiveWithVisited(srcFS, dstFS, path, make(map[string]bool), symlinks)
}

func (m *ISOModifier) copyFilesRecursiveWithVisited(srcFS, dstFS filesystem.FileSystem, path string, visited map[string]bool, symlinks map[string]bool) error {
	// Prevent infinite loops from symlinks like "debian -> ."
	if visited[path] {
		return nil
	}
	visited[path] = true

	files, err := srcFS.ReadDir(path)
	if err != nil {
		return err
	}

	for _, fi := range files {
		srcPath := filepath.Join(path, fi.Name())
		srcPath = filepath.ToSlash(srcPath)

		// Track symlinks for later resolution
		// go-diskfs can't create symlinks in ISO9660, so we need to resolve them
		if fi.Mode()&os.ModeSymlink != 0 {
			symlinks[srcPath] = true
			continue // Skip for now, will be resolved in resolveSymlinks
		}

		if fi.IsDir() {
			// Create directory
			if err := m.mkdirAll(dstFS, srcPath); err != nil {
				return err
			}
			// Recurse
			if err := m.copyFilesRecursiveWithVisited(srcFS, dstFS, srcPath, visited, symlinks); err != nil {
				return err
			}
		} else {
			// Skip if we're going to overwrite with new content
			cleanPath := strings.TrimPrefix(srcPath, "/")
			if _, exists := m.files[cleanPath]; exists {
				continue
			}

			// Copy file
			srcFile, err := srcFS.OpenFile(srcPath, os.O_RDONLY)
			if err != nil {
				return fmt.Errorf("failed to open source file %s: %w", srcPath, err)
			}

			content, err := io.ReadAll(srcFile)
			srcFile.Close()
			if err != nil {
				return fmt.Errorf("failed to read source file %s: %w", srcPath, err)
			}

			dstFile, err := dstFS.OpenFile(srcPath, os.O_CREATE|os.O_WRONLY)
			if err != nil {
				return fmt.Errorf("failed to create destination file %s: %w", srcPath, err)
			}

			_, err = dstFile.Write(content)
			dstFile.Close()
			if err != nil {
				return fmt.Errorf("failed to write destination file %s: %w", srcPath, err)
			}
		}
	}

	return nil
}

// resolveSymlinks attempts to resolve symlinks by detecting their targets and copying content
// Since go-diskfs doesn't expose symlink targets, we use heuristics for common patterns
func (m *ISOModifier) resolveSymlinks(srcFS, dstFS filesystem.FileSystem, symlinks map[string]bool) error {
	for symlinkPath := range symlinks {
		// Skip root-pointing symlinks like /debian -> . to avoid infinite recursion
		if symlinkPath == "/debian" {
			continue
		}

		// Try to detect symlink target using common patterns
		target := m.detectSymlinkTarget(srcFS, symlinkPath)
		if target == "" {
			// Can't determine target, skip this symlink
			continue
		}

		// Check if target exists and is a directory
		entries, err := srcFS.ReadDir(target)
		if err != nil || len(entries) == 0 {
			// Target doesn't exist or is empty, skip
			continue
		}

		// Create the symlink path as a real directory
		if err := m.mkdirAll(dstFS, symlinkPath); err != nil {
			return fmt.Errorf("failed to create symlink directory %s: %w", symlinkPath, err)
		}

		// Copy target contents to symlink path
		if err := m.copyDirectoryContents(srcFS, dstFS, target, symlinkPath); err != nil {
			return fmt.Errorf("failed to copy symlink contents from %s to %s: %w", target, symlinkPath, err)
		}
	}

	return nil
}

// detectSymlinkTarget attempts to determine the target of a symlink using common patterns
func (m *ISOModifier) detectSymlinkTarget(srcFS filesystem.FileSystem, symlinkPath string) string {
	dir := filepath.Dir(symlinkPath)
	name := filepath.Base(symlinkPath)

	// Common Debian/Ubuntu patterns
	switch {
	case dir == "/dists" && (name == "stable" || name == "testing" || name == "unstable" || name == "oldstable"):
		// /dists/stable -> bookworm, /dists/testing -> trixie, etc.
		// Try to find the actual release directory
		entries, err := srcFS.ReadDir(dir)
		if err != nil {
			return ""
		}
		// Common release codenames
		codenames := []string{"bookworm", "bullseye", "buster", "stretch", "jessie",
			"trixie", "forky", "sid", "noble", "jammy", "focal", "bionic"}
		for _, entry := range entries {
			if entry.IsDir() && entry.Mode()&os.ModeSymlink == 0 {
				entryName := entry.Name()
				for _, codename := range codenames {
					if entryName == codename {
						return filepath.Join(dir, entryName)
					}
				}
			}
		}
	}

	// For other symlinks, try to find a sibling with the same prefix
	// This handles cases like /doc/FAQ/html/index.html -> index.en.html
	entries, err := srcFS.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if entry.Mode()&os.ModeSymlink == 0 && strings.HasPrefix(entry.Name(), name+".") {
			return filepath.Join(dir, entry.Name())
		}
	}

	return ""
}

// copyDirectoryContents copies all contents from srcDir to dstDir
func (m *ISOModifier) copyDirectoryContents(srcFS, dstFS filesystem.FileSystem, srcDir, dstDir string) error {
	entries, err := srcFS.ReadDir(srcDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(srcDir, entry.Name())
		dstPath := filepath.Join(dstDir, entry.Name())
		srcPath = filepath.ToSlash(srcPath)
		dstPath = filepath.ToSlash(dstPath)

		// Skip symlinks in the copy
		if entry.Mode()&os.ModeSymlink != 0 {
			continue
		}

		if entry.IsDir() {
			if err := m.mkdirAll(dstFS, dstPath); err != nil {
				return err
			}
			if err := m.copyDirectoryContents(srcFS, dstFS, srcPath, dstPath); err != nil {
				return err
			}
		} else {
			// Skip if we're going to overwrite with new content
			cleanPath := strings.TrimPrefix(dstPath, "/")
			if _, exists := m.files[cleanPath]; exists {
				continue
			}

			srcFile, err := srcFS.OpenFile(srcPath, os.O_RDONLY)
			if err != nil {
				return fmt.Errorf("failed to open source file %s: %w", srcPath, err)
			}

			content, err := io.ReadAll(srcFile)
			srcFile.Close()
			if err != nil {
				return fmt.Errorf("failed to read source file %s: %w", srcPath, err)
			}

			dstFile, err := dstFS.OpenFile(dstPath, os.O_CREATE|os.O_WRONLY)
			if err != nil {
				return fmt.Errorf("failed to create destination file %s: %w", dstPath, err)
			}

			_, err = dstFile.Write(content)
			dstFile.Close()
			if err != nil {
				return fmt.Errorf("failed to write destination file %s: %w", dstPath, err)
			}
		}
	}

	return nil
}

func (m *ISOModifier) mkdirAll(fs filesystem.FileSystem, path string) error {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	current := ""

	for _, part := range parts {
		current = current + "/" + part
		// Try to create - ignore error if already exists
		fs.Mkdir(current)
	}

	return nil
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

// patchBootInfoTable patches the boot-info-table into an isolinux boot image
// This is required for isolinux to boot correctly from the ISO.
// The boot-info-table format (at offset 8 in the boot image):
//   - Offset 8-11:  bi_pvd    - LBA of primary volume descriptor (always 16)
//   - Offset 12-15: bi_file   - LBA of boot file
//   - Offset 16-19: bi_length - Length of boot file in bytes
//   - Offset 20-55: bi_csum   - Checksum (sum of 32-bit words from offset 64 to EOF)
func (m *ISOModifier) patchBootInfoTable(isoPath, bootImagePath string) error {
	const sectorSize = 2048
	const pvdLBA = 16 // Primary Volume Descriptor is always at sector 16

	// Open ISO for reading and writing
	f, err := os.OpenFile(isoPath, os.O_RDWR, 0644)
	if err != nil {
		return fmt.Errorf("failed to open ISO for patching: %w", err)
	}
	defer f.Close()

	// Find the boot image file's LBA by parsing the El Torito boot catalog
	bootFileLBA, bootFileLen, err := m.findBootFileLBA(f, bootImagePath)
	if err != nil {
		return fmt.Errorf("failed to find boot file LBA: %w", err)
	}

	// Read the boot image from the ISO
	bootImageOffset := int64(bootFileLBA) * sectorSize
	bootImage := make([]byte, bootFileLen)
	_, err = f.ReadAt(bootImage, bootImageOffset)
	if err != nil {
		return fmt.Errorf("failed to read boot image: %w", err)
	}

	// Calculate checksum (sum of 32-bit words from offset 64 to end)
	var checksum uint32
	for i := 64; i+4 <= len(bootImage); i += 4 {
		checksum += binary.LittleEndian.Uint32(bootImage[i : i+4])
	}

	// Debug: log boot-info-table values
	fmt.Printf("  Boot-info-table patch: LBA=%d, len=%d, offset=%d, checksum=0x%08x\n",
		bootFileLBA, bootFileLen, bootImageOffset, checksum)

	// Patch the boot-info-table at offset 8
	binary.LittleEndian.PutUint32(bootImage[8:12], pvdLBA)            // bi_pvd
	binary.LittleEndian.PutUint32(bootImage[12:16], bootFileLBA)      // bi_file
	binary.LittleEndian.PutUint32(bootImage[16:20], uint32(bootFileLen)) // bi_length
	binary.LittleEndian.PutUint32(bootImage[20:24], checksum)         // bi_csum

	// Write the patched boot image back to the ISO
	_, err = f.WriteAt(bootImage, bootImageOffset)
	if err != nil {
		return fmt.Errorf("failed to write patched boot image: %w", err)
	}

	return nil
}

// findBootFileLBA finds the LBA and length of the boot file by parsing the ISO
func (m *ISOModifier) findBootFileLBA(f *os.File, bootImagePath string) (uint32, int, error) {
	const sectorSize = 2048

	// Read the El Torito Boot Record Volume Descriptor (sector 17)
	bootRecordSector := make([]byte, sectorSize)
	_, err := f.ReadAt(bootRecordSector, 17*sectorSize)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to read boot record: %w", err)
	}

	// Verify it's a boot record (type 0, "CD001", version 1, "EL TORITO")
	if bootRecordSector[0] != 0 || string(bootRecordSector[1:6]) != "CD001" {
		return 0, 0, fmt.Errorf("invalid boot record descriptor")
	}
	if string(bootRecordSector[7:39]) != "EL TORITO SPECIFICATION" {
		// Try finding boot catalog LBA directly from the record
	}

	// Get boot catalog LBA (little-endian at offset 71)
	bootCatalogLBA := binary.LittleEndian.Uint32(bootRecordSector[71:75])

	// Read boot catalog
	bootCatalog := make([]byte, sectorSize)
	_, err = f.ReadAt(bootCatalog, int64(bootCatalogLBA)*sectorSize)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to read boot catalog: %w", err)
	}

	// Parse validation entry (first 32 bytes)
	if bootCatalog[0] != 1 { // Header ID
		return 0, 0, fmt.Errorf("invalid boot catalog validation entry")
	}

	// Parse initial/default entry (next 32 bytes, at offset 32)
	defaultEntry := bootCatalog[32:64]
	if defaultEntry[0] != 0x88 { // Bootable
		return 0, 0, fmt.Errorf("default boot entry not bootable")
	}

	// Get boot file LBA (little-endian at offset 8 of entry)
	bootFileLBA := binary.LittleEndian.Uint32(defaultEntry[8:12])

	// Always read actual file size from directory structure.
	// El Torito sector count is unreliable - it often gives a small value
	// (e.g., 4 sectors = 2048 bytes) while the actual file is much larger
	// (e.g., isolinux.bin is ~40KB).
	bootFileLen, err := m.findFileSizeFromDirectory(f, bootImagePath)
	if err != nil {
		// Fallback: use El Torito sector count
		sectorCount := binary.LittleEndian.Uint16(defaultEntry[6:8])
		bootFileLen = int(sectorCount) * 512
		if bootFileLen < 2048 {
			// Use a default reasonable size for isolinux
			bootFileLen = 64 * 1024 // 64KB default
		}
	}

	return bootFileLBA, bootFileLen, nil
}

// findFileSizeFromDirectory finds a file's size by parsing the ISO directory structure
func (m *ISOModifier) findFileSizeFromDirectory(f *os.File, filePath string) (int, error) {
	const sectorSize = 2048

	// Read Primary Volume Descriptor (sector 16)
	pvd := make([]byte, sectorSize)
	_, err := f.ReadAt(pvd, 16*sectorSize)
	if err != nil {
		return 0, err
	}

	// Root directory record starts at offset 156 in PVD
	rootDirRecord := pvd[156:190]
	rootDirLBA := binary.LittleEndian.Uint32(rootDirRecord[2:6])
	rootDirLen := binary.LittleEndian.Uint32(rootDirRecord[10:14])

	// Parse the path
	parts := strings.Split(strings.Trim(filePath, "/"), "/")

	currentLBA := rootDirLBA
	currentLen := rootDirLen

	// Navigate through directories
	for i, part := range parts {
		isLast := i == len(parts)-1
		found := false

		// Read directory
		dirData := make([]byte, currentLen)
		_, err := f.ReadAt(dirData, int64(currentLBA)*sectorSize)
		if err != nil {
			return 0, err
		}

		// Parse directory entries
		offset := 0
		for offset < len(dirData) {
			recordLen := int(dirData[offset])
			if recordLen == 0 {
				// Padding, move to next sector
				offset = ((offset / sectorSize) + 1) * sectorSize
				if offset >= len(dirData) {
					break
				}
				continue
			}

			nameLen := int(dirData[offset+32])
			if nameLen > 0 && offset+33+nameLen <= len(dirData) {
				name := string(dirData[offset+33 : offset+33+nameLen])
				// Remove version number (;1)
				if idx := strings.Index(name, ";"); idx > 0 {
					name = name[:idx]
				}
				// Remove trailing dot
				name = strings.TrimSuffix(name, ".")

				if strings.EqualFold(name, part) {
					if isLast {
						// Found the file, return its size
						fileSize := binary.LittleEndian.Uint32(dirData[offset+10 : offset+14])
						return int(fileSize), nil
					} else {
						// It's a directory, descend into it
						currentLBA = binary.LittleEndian.Uint32(dirData[offset+2 : offset+6])
						currentLen = binary.LittleEndian.Uint32(dirData[offset+10 : offset+14])
						found = true
						break
					}
				}
			}

			offset += recordLen
		}

		if !found && !isLast {
			return 0, fmt.Errorf("directory not found: %s", part)
		}
	}

	return 0, fmt.Errorf("file not found: %s", filePath)
}

// isUDFFilesystem checks if the ISO uses UDF filesystem (common for Windows ISOs)
func (m *ISOModifier) isUDFFilesystem() (bool, error) {
	f, err := os.Open(m.sourcePath)
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

// copyFile copies a file from src to dst
func (m *ISOModifier) copyFile(src, dst string) error {
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
