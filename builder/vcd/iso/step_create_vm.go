package iso

import (
	"context"
	"fmt"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/juanfont/packer-plugin-vcd/builder/vcd/driver"
	"github.com/vmware/go-vcloud-director/v3/govcd"
	"github.com/vmware/go-vcloud-director/v3/types/v56"
)

type StepCreateVM struct {
	VMName           string
	Description      string
	StorageProfile   string
	Network          string
	IPAllocationMode string
	VMIPAddress      string
	GuestOSType      string
	Firmware         string
	HardwareVersion  string
	DiskSizeMB       int64
}

func (s *StepCreateVM) Run(_ context.Context, state multistep.StateBag) multistep.StepAction {
	ui := state.Get("ui").(packersdk.Ui)
	d := state.Get("driver").(driver.Driver)
	vapp := state.Get("vapp").(*govcd.VApp)
	vdc := state.Get("vdc").(*govcd.Vdc)

	ui.Sayf("Creating VM: %s", s.VMName)

	// Get storage profile reference
	var storageProfileRef *types.Reference
	if s.StorageProfile != "" {
		sp, err := vdc.FindStorageProfileReference(s.StorageProfile)
		if err != nil {
			state.Put("error", fmt.Errorf("error finding storage profile %s: %w", s.StorageProfile, err))
			return multistep.ActionHalt
		}
		storageProfileRef = &sp
	}

	// Get the computer name from VM name (sanitized)
	computerName := s.VMName
	if len(computerName) > 15 {
		computerName = computerName[:15]
	}

	// Determine boot firmware
	firmware := "bios"
	if s.Firmware != "" {
		firmware = s.Firmware
	}

	// Determine hardware version (default to vmx-21 for ESXi 8.0+)
	hwVersion := "vmx-21"
	if s.HardwareVersion != "" {
		hwVersion = s.HardwareVersion
	}

	// Create empty VM parameters
	emptyVmParams := &types.RecomposeVAppParamsForEmptyVm{
		XmlnsVcloud: types.XMLNamespaceVCloud,
		XmlnsOvf:    types.XMLNamespaceOVF,
		CreateItem: &types.CreateItem{
			Name:                      s.VMName,
			Description:               s.Description,
			GuestCustomizationSection: nil,
			VmSpecSection: &types.VmSpecSection{
				Modified:          boolPointer(true),
				Info:              "Virtual Machine specification",
				OsType:            s.GuestOSType,
				NumCpus:           intPointer(1),           // Will be configured in hardware step
				NumCoresPerSocket: intPointer(1),           // Will be configured in hardware step
				CpuResourceMhz:    &types.CpuResourceMhz{}, // Let VCD decide
				MemoryResourceMb:  &types.MemoryResourceMb{Configured: 1024}, // Will be configured in hardware step
				MediaSection:      nil,                     // Media will be attached later
				DiskSection: &types.DiskSection{
					DiskSettings: []*types.DiskSettings{
						{
							SizeMb:            s.DiskSizeMB,
							UnitNumber:        0,
							BusNumber:         0,
							AdapterType:       "5", // LSI Logic SAS
							ThinProvisioned:   boolPointer(true),
							StorageProfile:    storageProfileRef,
							OverrideVmDefault: true,
						},
					},
				},
				HardwareVersion: &types.HardwareVersion{Value: hwVersion},
				VmToolsVersion:  "",
				VirtualCpuType:  "VM64",
				TimeSyncWithHost: boolPointer(false),
				Firmware:        firmware,
			},
			BootImage: nil,
		},
		AllEULAsAccepted: true,
	}

	// Add network connection if specified
	if s.Network != "" {
		ipAllocationMode := types.IPAllocationModePool
		if s.IPAllocationMode == "DHCP" {
			ipAllocationMode = types.IPAllocationModeDHCP
		} else if s.IPAllocationMode == "MANUAL" {
			ipAllocationMode = types.IPAllocationModeManual
		} else if s.IPAllocationMode == "NONE" {
			ipAllocationMode = types.IPAllocationModeNone
		}

		netConn := &types.NetworkConnection{
			Network:                 s.Network,
			NetworkConnectionIndex:  0,
			IsConnected:             true,
			IPAddressAllocationMode: ipAllocationMode,
			NetworkAdapterType:      "VMXNET3",
		}

		// Set static IP for MANUAL allocation mode
		if s.IPAllocationMode == "MANUAL" && s.VMIPAddress != "" {
			netConn.IPAddress = s.VMIPAddress
			ui.Sayf("Using static IP address: %s", s.VMIPAddress)
		}

		emptyVmParams.CreateItem.NetworkConnectionSection = &types.NetworkConnectionSection{
			PrimaryNetworkConnectionIndex: 0,
			NetworkConnection:             []*types.NetworkConnection{netConn},
		}
	}

	// Create the empty VM in the vApp
	vm, err := vapp.AddEmptyVm(emptyVmParams)
	if err != nil {
		state.Put("error", fmt.Errorf("error creating empty VM: %w", err))
		return multistep.ActionHalt
	}

	// Wrap in driver's VirtualMachine interface
	vmDriver := d.NewVM(vm)
	state.Put("vm", vmDriver)

	ui.Sayf("VM created: %s", s.VMName)
	return multistep.ActionContinue
}

func (s *StepCreateVM) Cleanup(state multistep.StateBag) {
	ui := state.Get("ui").(packersdk.Ui)

	// Only clean up on failure
	_, cancelled := state.GetOk(multistep.StateCancelled)
	_, halted := state.GetOk(multistep.StateHalted)
	if !cancelled && !halted {
		return
	}

	vmRaw, ok := state.GetOk("vm")
	if !ok {
		return
	}

	vm := vmRaw.(driver.VirtualMachine)
	vmName := vm.GetName()

	ui.Sayf("Deleting VM: %s", vmName)

	// Power off if needed
	if on, _ := vm.IsPoweredOn(); on {
		_ = vm.PowerOff()
	}

	// Delete the VM
	govcdVM := vm.GetVM()
	err := govcdVM.Delete()
	if err != nil {
		ui.Errorf("Error deleting VM: %s", err)
	}
}

func boolPointer(b bool) *bool {
	return &b
}

func intPointer(i int) *int {
	return &i
}
