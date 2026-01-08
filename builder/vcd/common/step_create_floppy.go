package common

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/tmp"
)

const (
	// FloppyMaxSize is the maximum size of a floppy image (1.44 MB)
	FloppyMaxSize = 1474560
)

// FloppyConfig contains the configuration for floppy image creation
type FloppyConfig struct {
	// Files to include on the floppy disk
	FloppyFiles []string `mapstructure:"floppy_files"`

	// Content to write to files on the floppy disk (path -> content)
	FloppyContent map[string]string `mapstructure:"floppy_content"`

	// Label for the floppy disk (max 11 characters)
	FloppyLabel string `mapstructure:"floppy_label"`
}

// StepCreateFloppy creates a floppy disk image with the given files.
type StepCreateFloppy struct {
	Files   []string
	Content map[string]string
	Label   string

	floppyPath string
	tempDir    string
}

func (s *StepCreateFloppy) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	if len(s.Files) == 0 && len(s.Content) == 0 {
		return multistep.ActionContinue
	}

	ui := state.Get("ui").(packersdk.Ui)
	ui.Say("Creating floppy disk image...")

	// Set default label
	if s.Label == "" {
		s.Label = "OEMDRV"
	}
	// Truncate label to 11 characters (FAT12 limit)
	if len(s.Label) > 11 {
		s.Label = s.Label[:11]
	}

	// Create temporary directory for staging files
	tempDir, err := tmp.Dir("packer_floppy")
	if err != nil {
		state.Put("error", fmt.Errorf("error creating temp directory for floppy: %s", err))
		return multistep.ActionHalt
	}
	s.tempDir = tempDir

	// Calculate total size of files to include
	totalSize := int64(0)

	// Copy files to temp directory and calculate size
	for _, file := range s.Files {
		size, err := s.addFile(tempDir, file)
		if err != nil {
			state.Put("error", fmt.Errorf("error adding file to floppy: %s", err))
			return multistep.ActionHalt
		}
		totalSize += size
	}

	// Write content to files
	for path, content := range s.Content {
		size, err := s.addContent(tempDir, path, content)
		if err != nil {
			state.Put("error", fmt.Errorf("error adding content to floppy: %s", err))
			return multistep.ActionHalt
		}
		totalSize += size
	}

	// Check size limit
	if totalSize > FloppyMaxSize {
		state.Put("error", fmt.Errorf(
			"floppy content size (%d bytes) exceeds maximum floppy size (%d bytes / 1.44 MB). "+
				"Consider using cd_files/cd_content instead for larger content",
			totalSize, FloppyMaxSize))
		return multistep.ActionHalt
	}

	ui.Sayf("Floppy content size: %d bytes (%.1f%% of 1.44 MB limit)",
		totalSize, float64(totalSize)/float64(FloppyMaxSize)*100)

	// Create floppy image file
	floppyFile, err := tmp.File("packer*.flp")
	if err != nil {
		state.Put("error", fmt.Errorf("error creating floppy image file: %s", err))
		return multistep.ActionHalt
	}
	s.floppyPath = floppyFile.Name()
	floppyFile.Close()
	os.Remove(s.floppyPath) // Remove so mkfs can create it

	// Create the floppy image
	err = s.createFloppyImage(ui)
	if err != nil {
		state.Put("error", fmt.Errorf("error creating floppy image: %s", err))
		return multistep.ActionHalt
	}

	// Copy files to the floppy image
	err = s.copyFilesToFloppy(ui, tempDir)
	if err != nil {
		state.Put("error", fmt.Errorf("error copying files to floppy: %s", err))
		return multistep.ActionHalt
	}

	ui.Sayf("Floppy disk image created: %s", s.floppyPath)
	state.Put("floppy_path", s.floppyPath)

	return multistep.ActionContinue
}

func (s *StepCreateFloppy) Cleanup(state multistep.StateBag) {
	if s.tempDir != "" {
		os.RemoveAll(s.tempDir)
	}
	if s.floppyPath != "" {
		os.Remove(s.floppyPath)
	}
}

// createFloppyImage creates an empty FAT12 floppy image
func (s *StepCreateFloppy) createFloppyImage(ui packersdk.Ui) error {
	// Try different tools to create the floppy image
	tools := []struct {
		name string
		args []string
	}{
		// mkfs.msdos / mkfs.vfat (Linux)
		{"mkfs.msdos", []string{"-C", s.floppyPath, "1440", "-n", s.Label}},
		{"mkfs.vfat", []string{"-C", s.floppyPath, "1440", "-n", s.Label}},
		// mformat (mtools)
		{"mformat", []string{"-C", "-f", "1440", "-v", s.Label, "-i", s.floppyPath, "::"}},
	}

	for _, tool := range tools {
		path, err := exec.LookPath(tool.name)
		if err != nil {
			continue
		}

		ui.Sayf("Using %s to create floppy image", tool.name)

		// For mformat, we need to create the file first
		if tool.name == "mformat" {
			f, err := os.Create(s.floppyPath)
			if err != nil {
				return err
			}
			// Create a 1.44MB file
			if err := f.Truncate(FloppyMaxSize); err != nil {
				f.Close()
				return err
			}
			f.Close()
		}

		cmd := exec.Command(path, tool.args...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			ui.Errorf("Error running %s: %s\nOutput: %s", tool.name, err, string(output))
			continue
		}
		return nil
	}

	return fmt.Errorf("no floppy creation tool found (tried: mkfs.msdos, mkfs.vfat, mformat). " +
		"Please install dosfstools or mtools")
}

// copyFilesToFloppy copies files from temp directory to the floppy image
func (s *StepCreateFloppy) copyFilesToFloppy(ui packersdk.Ui, sourceDir string) error {
	// Try mcopy (mtools) first, then fall back to mount+copy
	mcopyPath, err := exec.LookPath("mcopy")
	if err == nil {
		return s.copyWithMcopy(ui, mcopyPath, sourceDir)
	}

	// Try mounting the floppy (requires root)
	return s.copyWithMount(ui, sourceDir)
}

// copyWithMcopy uses mtools mcopy to copy files
func (s *StepCreateFloppy) copyWithMcopy(ui packersdk.Ui, mcopyPath, sourceDir string) error {
	// Walk the source directory and copy each file
	return filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Get relative path
		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}

		if relPath == "." {
			return nil
		}

		// Destination path on floppy (use :: for mtools)
		destPath := "::" + "/" + strings.ReplaceAll(relPath, string(filepath.Separator), "/")

		if info.IsDir() {
			// Create directory with mmd
			mmdPath, err := exec.LookPath("mmd")
			if err != nil {
				return fmt.Errorf("mmd not found: %s", err)
			}
			cmd := exec.Command(mmdPath, "-i", s.floppyPath, destPath)
			if output, err := cmd.CombinedOutput(); err != nil {
				// Ignore "directory already exists" errors
				if !strings.Contains(string(output), "already exist") {
					return fmt.Errorf("mmd error: %s: %s", err, string(output))
				}
			}
		} else {
			// Copy file
			cmd := exec.Command(mcopyPath, "-i", s.floppyPath, path, destPath)
			if output, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("mcopy error: %s: %s", err, string(output))
			}
			ui.Sayf("  Added: %s", relPath)
		}

		return nil
	})
}

// copyWithMount mounts the floppy and copies files (requires root)
func (s *StepCreateFloppy) copyWithMount(ui packersdk.Ui, sourceDir string) error {
	// Create mount point
	mountPoint, err := tmp.Dir("packer_floppy_mount")
	if err != nil {
		return err
	}
	defer os.RemoveAll(mountPoint)

	// Mount the floppy image
	mountCmd := exec.Command("mount", "-o", "loop", s.floppyPath, mountPoint)
	if output, err := mountCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to mount floppy (may require root): %s: %s", err, string(output))
	}

	// Ensure we unmount
	defer func() {
		umountCmd := exec.Command("umount", mountPoint)
		umountCmd.Run()
	}()

	// Copy files
	return filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}

		if relPath == "." {
			return nil
		}

		destPath := filepath.Join(mountPoint, relPath)

		if info.IsDir() {
			return os.MkdirAll(destPath, info.Mode())
		}

		// Copy file
		srcFile, err := os.Open(path)
		if err != nil {
			return err
		}
		defer srcFile.Close()

		dstFile, err := os.Create(destPath)
		if err != nil {
			return err
		}
		defer dstFile.Close()

		_, err = io.Copy(dstFile, srcFile)
		if err != nil {
			return err
		}

		ui.Sayf("  Added: %s", relPath)
		return nil
	})
}

// addFile copies a file or directory to the temp staging directory
func (s *StepCreateFloppy) addFile(dst, src string) (int64, error) {
	info, err := os.Stat(src)
	if err != nil {
		return 0, fmt.Errorf("error accessing file %s: %s", src, err)
	}

	if !info.IsDir() {
		// Copy single file
		return s.copyFile(dst, src, info)
	}

	// Copy directory recursively
	var totalSize int64
	err = filepath.Walk(src, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(filepath.Dir(src), path)
		if err != nil {
			return err
		}

		destPath := filepath.Join(dst, relPath)

		if fi.IsDir() {
			return os.MkdirAll(destPath, fi.Mode())
		}

		size, err := s.copyFile(filepath.Dir(destPath), path, fi)
		if err != nil {
			return err
		}
		totalSize += size
		return nil
	})

	return totalSize, err
}

// copyFile copies a single file to destination directory
func (s *StepCreateFloppy) copyFile(dstDir, srcPath string, info os.FileInfo) (int64, error) {
	srcFile, err := os.Open(srcPath)
	if err != nil {
		return 0, err
	}
	defer srcFile.Close()

	dstPath := filepath.Join(dstDir, info.Name())
	dstFile, err := os.Create(dstPath)
	if err != nil {
		return 0, err
	}
	defer dstFile.Close()

	n, err := io.Copy(dstFile, srcFile)
	return n, err
}

// addContent writes content to a file in the temp staging directory
func (s *StepCreateFloppy) addContent(dst, path, content string) (int64, error) {
	dstPath := filepath.Join(dst, path)

	// Create parent directories
	if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
		return 0, err
	}

	// Write content
	if err := os.WriteFile(dstPath, []byte(content), 0644); err != nil {
		return 0, err
	}

	return int64(len(content)), nil
}
