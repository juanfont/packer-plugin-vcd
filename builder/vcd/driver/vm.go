package driver

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/vmware/go-vcloud-director/v3/govcd"
	"github.com/vmware/go-vcloud-director/v3/types/v56"
)

var errPolicyNotFound = errors.New("VM sizing policy not found")

// VirtualMachine defines the interface for VM operations
type VirtualMachine interface {
	// Power operations
	PowerOn() error
	PowerOff() error
	Shutdown() error

	// Status
	GetStatus() (string, error)
	IsPoweredOn() (bool, error)
	IsPoweredOff() (bool, error)
	WaitForPowerOff(ctx context.Context, timeout time.Duration) error

	// Network
	GetIPAddress() (string, error)
	WaitForIP(ctx context.Context, timeout time.Duration) (string, error)
	ChangeIPAddress(newIP string) error

	// Media operations
	InsertMedia(catalogName, mediaName string) error
	EjectMedia(catalogName, mediaName string) error

	// Hardware configuration
	ChangeCPU(cpuCount, coresPerSocket int) error
	ChangeMemory(memoryMB int64) error
	SetTPM(enabled bool) error
	SetBootOptions(bootDelayMs int, efiSecureBoot bool) error

	// Info
	GetName() string
	GetVM() *govcd.VM
	Refresh() error
}

type VirtualMachineDriver struct {
	vm     *govcd.VM
	driver *VCDDriver
}

// --- Power Operations ---

func (v *VirtualMachineDriver) PowerOn() error {
	task, err := v.vm.PowerOn()
	if err != nil {
		return fmt.Errorf("error powering on VM: %w", err)
	}
	return task.WaitTaskCompletion()
}

func (v *VirtualMachineDriver) PowerOff() error {
	task, err := v.vm.PowerOff()
	if err != nil {
		return fmt.Errorf("error powering off VM: %w", err)
	}
	return task.WaitTaskCompletion()
}

func (v *VirtualMachineDriver) Shutdown() error {
	task, err := v.vm.Shutdown()
	if err != nil {
		return fmt.Errorf("error shutting down VM: %w", err)
	}
	return task.WaitTaskCompletion()
}

// --- Status Operations ---

func (v *VirtualMachineDriver) GetStatus() (string, error) {
	return v.vm.GetStatus()
}

func (v *VirtualMachineDriver) IsPoweredOn() (bool, error) {
	status, err := v.GetStatus()
	if err != nil {
		return false, err
	}
	return status == "POWERED_ON", nil
}

func (v *VirtualMachineDriver) IsPoweredOff() (bool, error) {
	status, err := v.GetStatus()
	if err != nil {
		return false, err
	}
	return status == "POWERED_OFF", nil
}

func (v *VirtualMachineDriver) WaitForPowerOff(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if time.Now().After(deadline) {
				return fmt.Errorf("timeout waiting for VM to power off")
			}

			off, err := v.IsPoweredOff()
			if err != nil {
				return fmt.Errorf("error checking VM power state: %w", err)
			}
			if off {
				return nil
			}
		}
	}
}

// --- Network Operations ---

// GetIPAddress returns the IP address of the primary NIC
func (v *VirtualMachineDriver) GetIPAddress() (string, error) {
	// Refresh VM to get latest network info
	if err := v.vm.Refresh(); err != nil {
		return "", fmt.Errorf("error refreshing VM: %w", err)
	}

	// Get network connection section which contains IP info from guest tools
	netSection, err := v.vm.GetNetworkConnectionSection()
	if err != nil {
		return "", fmt.Errorf("error getting network connection section: %w", err)
	}

	// Find the primary NIC using PrimaryNetworkConnectionIndex
	primaryIndex := netSection.PrimaryNetworkConnectionIndex

	for _, conn := range netSection.NetworkConnection {
		// Only use the primary NIC
		if conn.NetworkConnectionIndex != primaryIndex {
			continue
		}

		// Check for IP address reported by guest tools
		if conn.IPAddress != "" {
			return conn.IPAddress, nil
		}
		// Also check external IP (for NAT scenarios)
		if conn.ExternalIPAddress != "" {
			return conn.ExternalIPAddress, nil
		}
	}

	return "", nil // No IP found yet on primary NIC
}

// WaitForIP polls until the VM has an IP address or timeout
func (v *VirtualMachineDriver) WaitForIP(ctx context.Context, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-ticker.C:
			if time.Now().After(deadline) {
				return "", fmt.Errorf("timeout waiting for VM IP address")
			}

			ip, err := v.GetIPAddress()
			if err != nil {
				// Log error but continue polling
				continue
			}
			if ip != "" {
				return ip, nil
			}
		}
	}
}

// ChangeIPAddress updates the VM's primary NIC IP address
func (v *VirtualMachineDriver) ChangeIPAddress(newIP string) error {
	netSection, err := v.vm.GetNetworkConnectionSection()
	if err != nil {
		return fmt.Errorf("error getting network connection section: %w", err)
	}

	// Find the primary NIC and update its IP
	primaryIndex := netSection.PrimaryNetworkConnectionIndex
	for i, conn := range netSection.NetworkConnection {
		if conn.NetworkConnectionIndex == primaryIndex {
			netSection.NetworkConnection[i].IPAddress = newIP
			break
		}
	}

	err = v.vm.UpdateNetworkConnectionSection(netSection)
	if err != nil {
		return fmt.Errorf("error updating network connection: %w", err)
	}

	return nil
}

// --- Media Operations ---

func (v *VirtualMachineDriver) InsertMedia(catalogName, mediaName string) error {
	org, err := v.driver.GetOrg()
	if err != nil {
		return err
	}

	// First, get the catalog and check media status
	catalog, err := org.GetCatalogByName(catalogName, true)
	if err != nil {
		return fmt.Errorf("error getting catalog %s: %w", catalogName, err)
	}

	// Wait for media to be RESOLVED (status = 1)
	maxStatusRetries := 30
	statusRetryDelay := 10 * time.Second

	for i := 0; i < maxStatusRetries; i++ {
		media, err := catalog.GetMediaByName(mediaName, true)
		if err != nil {
			return fmt.Errorf("error getting media %s: %w", mediaName, err)
		}

		if media.Media.Status == 1 { // RESOLVED
			break
		}

		fmt.Printf("Media status is %d (need 1=RESOLVED), waiting %v... (%d/%d)\n",
			media.Media.Status, statusRetryDelay, i+1, maxStatusRetries)

		if i == maxStatusRetries-1 {
			return fmt.Errorf("media %s never reached RESOLVED status (current: %d)", mediaName, media.Media.Status)
		}
		time.Sleep(statusRetryDelay)
	}

	// Retry logic for media insertion
	maxRetries := 12
	retryDelay := 30 * time.Second

	var lastErr error
	for i := 0; i < maxRetries; i++ {
		task, err := v.vm.HandleInsertMedia(org, catalogName, mediaName)
		if err == nil {
			return task.WaitTaskCompletion()
		}

		// Check if it's a 409 state error
		if strings.Contains(err.Error(), "409") || strings.Contains(err.Error(), "not supported in the current state") {
			lastErr = err
			fmt.Printf("Media insert attempt %d/%d failed (state error), waiting %v before retry...\n", i+1, maxRetries, retryDelay)
			time.Sleep(retryDelay)
			continue
		}

		// Non-retryable error
		return fmt.Errorf("error inserting media %s: %w", mediaName, err)
	}

	return fmt.Errorf("error inserting media %s after %d retries: %w", mediaName, maxRetries, lastErr)
}

func (v *VirtualMachineDriver) EjectMedia(catalogName, mediaName string) error {
	org, err := v.driver.GetOrg()
	if err != nil {
		return err
	}

	_, err = v.vm.HandleEjectMediaAndAnswer(org, catalogName, mediaName, true)
	if err != nil {
		return fmt.Errorf("error ejecting media %s: %w", mediaName, err)
	}
	return nil
}

// --- Hardware Configuration ---

func (v *VirtualMachineDriver) ChangeCPU(cpuCount, coresPerSocket int) error {
	err := v.vm.ChangeCPUAndCoreCount(&cpuCount, &coresPerSocket)
	if err != nil {
		return fmt.Errorf("error changing CPU: %w", err)
	}
	return nil
}

func (v *VirtualMachineDriver) ChangeMemory(memoryMB int64) error {
	err := v.vm.ChangeMemory(memoryMB)
	if err != nil {
		return fmt.Errorf("error changing memory: %w", err)
	}
	return nil
}

func (v *VirtualMachineDriver) SetTPM(enabled bool) error {
	tpmEdit := &TrustedPlatformModuleEdit{
		Xmlns:      types.XMLNamespaceVCloud,
		TpmPresent: enabled,
	}

	task, err := v.driver.client.Client.ExecuteTaskRequest(
		v.vm.VM.HREF+"/action/editTrustedPlatformModule",
		http.MethodPost,
		"application/vnd.vmware.vcloud.TpmSection+xml",
		"error changing TPM for VM: %s",
		tpmEdit,
	)
	if err != nil {
		return fmt.Errorf("error setting TPM: %w", err)
	}

	return task.WaitTaskCompletion()
}

func (v *VirtualMachineDriver) SetBootOptions(bootDelayMs int, efiSecureBoot bool) error {
	bootOptions := &types.BootOptions{}

	if bootDelayMs > 0 {
		bootOptions.BootDelay = &bootDelayMs
	}

	if efiSecureBoot {
		bootOptions.EfiSecureBootEnabled = boolPtr(true)
	}

	_, err := v.vm.UpdateBootOptions(bootOptions)
	if err != nil {
		return fmt.Errorf("error setting boot options: %w", err)
	}

	return nil
}

func boolPtr(b bool) *bool {
	return &b
}

// --- Info ---

func (v *VirtualMachineDriver) GetName() string {
	return v.vm.VM.Name
}

func (v *VirtualMachineDriver) GetVM() *govcd.VM {
	return v.vm
}

func (v *VirtualMachineDriver) Refresh() error {
	return v.vm.Refresh()
}

// --- Compute Policy Operations ---

// GetVMSizingPolicyByName finds a VM sizing policy by name from a list of policies
func GetVMSizingPolicyByName(sizingPolicies []*govcd.VdcComputePolicyV2, policyName string) (*govcd.VdcComputePolicyV2, error) {
	for _, policy := range sizingPolicies {
		if policy.VdcComputePolicyV2.Name == policyName {
			return policy, nil
		}
	}
	return nil, errPolicyNotFound
}
