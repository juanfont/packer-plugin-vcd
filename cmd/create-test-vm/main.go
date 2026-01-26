package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/juanfont/packer-plugin-vcd/builder/vcd/driver"
	"github.com/vmware/go-vcloud-director/v3/types/v56"
)

func main() {
	// Get credentials from environment
	host := os.Getenv("VCD_HOST")
	username := os.Getenv("VCD_USERNAME")
	password := os.Getenv("VCD_PASSWORD")
	org := os.Getenv("VCD_ORG")
	vdcName := os.Getenv("VCD_VDC")
	network := os.Getenv("VCD_NETWORK")
	storageProfile := os.Getenv("VCD_STORAGE_PROFILE")
	isoPath := os.Getenv("ISO_PATH")

	if host == "" || username == "" || password == "" || org == "" || vdcName == "" {
		log.Fatal("Missing required environment variables: VCD_HOST, VCD_USERNAME, VCD_PASSWORD, VCD_ORG, VCD_VDC")
	}

	if isoPath == "" {
		isoPath = "/tmp/iso-min-test/win11-minimal-test.iso"
	}
	if network == "" {
		network = "FCI-IRT_ISN6_ORG-SVC"
	}
	if storageProfile == "" {
		storageProfile = "ESR-Tier-3"
	}

	// Connect to VCD
	fmt.Println("Connecting to VCD...")
	d, err := driver.NewDriver(&driver.ConnectConfig{
		Host:               host,
		Org:                org,
		Username:           username,
		Password:           password,
		InsecureConnection: true,
	})
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	fmt.Println("Connected!")

	// Get VDC
	vdc, err := d.GetVdc(vdcName)
	if err != nil {
		log.Fatalf("Failed to get VDC: %v", err)
	}

	// Get storage profile reference
	storageProfileRef, err := vdc.FindStorageProfileReference(storageProfile)
	if err != nil {
		log.Fatalf("Failed to find storage profile %s: %v", storageProfile, err)
	}

	// Create catalog for ISO
	catalogName := fmt.Sprintf("win11-test-%d", time.Now().Unix())
	fmt.Printf("Creating catalog: %s\n", catalogName)

	_, err = d.CreateCatalogWithStorageProfile(catalogName, "Test catalog", &storageProfileRef)
	if err != nil {
		log.Fatalf("Failed to create catalog: %v", err)
	}
	fmt.Printf("Catalog created: %s\n", catalogName)

	// Get catalog for media upload (need Catalog not AdminCatalog)
	catForUpload, err := d.GetCatalog(catalogName)
	if err != nil {
		log.Fatalf("Failed to get catalog for upload: %v", err)
	}

	// Upload ISO
	fmt.Printf("Uploading ISO: %s (this may take several minutes)...\n", isoPath)
	_, err = d.UploadMediaImage(catForUpload, "win11-test.iso", "Windows 11 test ISO", isoPath)
	if err != nil {
		log.Fatalf("Failed to upload ISO: %v", err)
	}
	fmt.Println("ISO uploaded!")

	// Create vApp
	vappName := fmt.Sprintf("win11-test-%d", time.Now().Unix())
	fmt.Printf("Creating vApp: %s\n", vappName)

	vapp, err := d.CreateVApp(vdc, vappName, "Windows 11 test vApp", network)
	if err != nil {
		log.Fatalf("Failed to create vApp: %v", err)
	}
	fmt.Printf("vApp created: %s\n", vappName)

	// Create empty VM
	vmName := "win11-console-check"
	fmt.Printf("Creating VM: %s\n", vmName)

	emptyVmParams := &types.RecomposeVAppParamsForEmptyVm{
		XmlnsVcloud: types.XMLNamespaceVCloud,
		XmlnsOvf:    types.XMLNamespaceOVF,
		CreateItem: &types.CreateItem{
			Name:        vmName,
			Description: "Windows 11 test VM",
			StorageProfile: &storageProfileRef,
			VmSpecSection: &types.VmSpecSection{
				Modified:          boolPtr(true),
				Info:              "Virtual Machine specification",
				OsType:            "windows2019srvNext_64Guest",
				NumCpus:           intPtr(4),
				NumCoresPerSocket: intPtr(1),
				CpuResourceMhz:    &types.CpuResourceMhz{},
				MemoryResourceMb:  &types.MemoryResourceMb{Configured: 8192},
				DiskSection: &types.DiskSection{
					DiskSettings: []*types.DiskSettings{
						{
							SizeMb:            65536,
							UnitNumber:        0,
							BusNumber:         0,
							AdapterType:       "5", // LSI Logic SAS
							ThinProvisioned:   boolPtr(true),
							StorageProfile:    &storageProfileRef,
							OverrideVmDefault: true,
						},
					},
				},
				HardwareVersion:  &types.HardwareVersion{Value: "vmx-21"},
				VirtualCpuType:   "VM64",
				TimeSyncWithHost: boolPtr(false),
				Firmware:         "efi",
			},
			NetworkConnectionSection: &types.NetworkConnectionSection{
				PrimaryNetworkConnectionIndex: 0,
				NetworkConnection: []*types.NetworkConnection{
					{
						Network:                 network,
						NetworkConnectionIndex:  0,
						IsConnected:             true,
						IPAddressAllocationMode: types.IPAllocationModePool,
						NetworkAdapterType:      "VMXNET3",
					},
				},
			},
		},
		AllEULAsAccepted: true,
	}

	govcdVM, err := vapp.AddEmptyVm(emptyVmParams)
	if err != nil {
		log.Fatalf("Failed to create VM: %v", err)
	}
	vm := d.NewVM(govcdVM)
	fmt.Printf("VM created: %s\n", vmName)

	// Enable TPM
	fmt.Println("Enabling TPM...")
	err = vm.SetTPM(true)
	if err != nil {
		log.Printf("Warning: Failed to enable TPM: %v", err)
	} else {
		fmt.Println("TPM enabled!")
	}

	// Mount ISO
	fmt.Println("Mounting ISO (waiting for media to be ready)...")
	err = vm.InsertMedia(catalogName, "win11-test.iso")
	if err != nil {
		log.Fatalf("Failed to mount ISO: %v", err)
	}
	fmt.Println("ISO mounted!")

	// Power on
	fmt.Println("Powering on VM...")
	err = vm.PowerOn()
	if err != nil {
		log.Fatalf("Failed to power on VM: %v", err)
	}
	fmt.Println("VM powered on!")

	// Wait a moment for boot
	fmt.Println("Waiting 10s for VM to start booting...")
	time.Sleep(10 * time.Second)

	// Try to send spacebar to trigger "Press any key to boot from CD"
	fmt.Println("Attempting to send boot key (spacebar)...")
	client := d.GetClient()
	ticket, err := driver.AcquireMksTicket(client, govcdVM)
	if err != nil {
		log.Printf("Warning: Failed to acquire MKS ticket: %v", err)
	} else {
		wmks := driver.NewWMKSClient(ticket, driver.WithInsecure(true))
		err = wmks.Connect()
		if err != nil {
			log.Printf("Warning: Failed to connect to WMKS: %v", err)
		} else {
			err = wmks.SendSpecialKey("spacebar")
			if err != nil {
				log.Printf("Warning: Failed to send boot key: %v", err)
			} else {
				fmt.Println("Boot key sent!")
			}
			wmks.Close()
		}
	}

	fmt.Println("")
	fmt.Println("========================================")
	fmt.Println("VM is running! Check the VCD console:")
	fmt.Printf("  vApp: %s\n", vappName)
	fmt.Printf("  VM:   %s\n", vmName)
	fmt.Println("")
	fmt.Println("Resources created (clean up manually):")
	fmt.Printf("  Catalog: %s\n", catalogName)
	fmt.Printf("  vApp:    %s\n", vappName)
	fmt.Println("========================================")
}

func boolPtr(b bool) *bool { return &b }
func intPtr(i int) *int    { return &i }
