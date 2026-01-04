// Test script for VCD connectivity and catalog creation
package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/juanfont/packer-plugin-vcd/builder/vcd/driver"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/vmware/go-vcloud-director/v3/types/v56"
)

var rootCmd = &cobra.Command{
	Use:   "vcdtest",
	Short: "Test VCD connectivity and catalog creation",
	Run:   runTest,
}

var uploadISOCmd = &cobra.Command{
	Use:   "upload-iso [iso-path]",
	Short: "Upload an ISO to a temporary catalog",
	Args:  cobra.ExactArgs(1),
	Run:   runUploadISO,
}

var listNetworksCmd = &cobra.Command{
	Use:   "list-networks",
	Short: "List available networks in the VDC",
	Run:   runListNetworks,
}

var createVMCmd = &cobra.Command{
	Use:   "create-vm",
	Short: "Create a VM and mount the ISO",
	Run:   runCreateVM,
}

var fullTestCmd = &cobra.Command{
	Use:   "full-test [iso-path]",
	Short: "Full test: create catalog with VDC storage, upload ISO, create VM, mount ISO",
	Args:  cobra.ExactArgs(1),
	Run:   runFullTest,
}

var consoleTestCmd = &cobra.Command{
	Use:   "console-test [vm-href]",
	Short: "Test WMKS console connection and send keystrokes",
	Args:  cobra.ExactArgs(1),
	Run:   runConsoleTest,
}

func init() {
	viper.SetEnvPrefix("")
	viper.AutomaticEnv()

	// Also load from .env file if present
	viper.SetConfigFile(".env")
	viper.SetConfigType("env")
	viper.ReadInConfig()

	rootCmd.AddCommand(uploadISOCmd)
	rootCmd.AddCommand(listNetworksCmd)
	rootCmd.AddCommand(createVMCmd)
	rootCmd.AddCommand(fullTestCmd)
	rootCmd.AddCommand(consoleTestCmd)

	// Flags for console-test
	consoleTestCmd.Flags().String("text", "hello", "Text to type via console")
	consoleTestCmd.Flags().Bool("enter", false, "Press Enter after text")

	// Flags for create-vm
	createVMCmd.Flags().String("catalog", "packer-iso-test-1767538918", "Catalog containing the ISO")
	createVMCmd.Flags().String("iso", "debian-12.12.0-amd64-netinst.iso", "ISO media name")
	createVMCmd.Flags().String("vm-name", "packer-test-vm", "Name for the VM")
	createVMCmd.Flags().String("vapp", "", "vApp name (created if empty)")
	createVMCmd.Flags().String("network", "", "Network to attach")
	createVMCmd.Flags().String("storage-profile", "", "Storage profile")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func getDriver() (driver.Driver, error) {
	host := viper.GetString("VCD_HOST")
	username := viper.GetString("VCD_USERNAME")
	password := viper.GetString("VCD_PASSWORD")
	org := viper.GetString("VCD_ORG")
	insecure := viper.GetString("VCD_VERIFY_SSL") == "false"

	if host == "" || username == "" || password == "" || org == "" {
		return nil, fmt.Errorf("missing required environment variables: VCD_HOST, VCD_USERNAME, VCD_PASSWORD, VCD_ORG")
	}

	// Strip https:// prefix if present - driver adds it
	host = strings.TrimPrefix(host, "https://")
	host = strings.TrimPrefix(host, "http://")

	fmt.Printf("Connecting to VCD:\n")
	fmt.Printf("  Host: %s\n", host)
	fmt.Printf("  Org: %s\n", org)
	fmt.Printf("  User: %s\n", username)
	fmt.Printf("  Insecure: %v\n", insecure)

	config := &driver.ConnectConfig{
		Host:               host,
		Org:                org,
		Username:           username,
		Password:           password,
		InsecureConnection: insecure,
	}

	return driver.NewDriver(config)
}

func runUploadISO(cmd *cobra.Command, args []string) {
	isoPath := args[0]

	// Check if file exists
	if _, err := os.Stat(isoPath); os.IsNotExist(err) {
		fmt.Printf("Error: ISO file not found: %s\n", isoPath)
		os.Exit(1)
	}

	fmt.Printf("ISO file: %s\n\n", isoPath)

	d, err := getDriver()
	if err != nil {
		fmt.Printf("Connection failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Connection successful!")

	// Create temporary catalog
	tempCatalogName := fmt.Sprintf("packer-iso-test-%d", time.Now().Unix())
	fmt.Printf("\nCreating temporary catalog: %s...\n", tempCatalogName)

	adminCatalog, err := d.CreateCatalog(tempCatalogName, "Temporary catalog for ISO upload test")
	if err != nil {
		fmt.Printf("Error creating catalog: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Catalog created: %s\n", adminCatalog.AdminCatalog.Name)

	// Get regular catalog reference for upload
	catalog, err := d.GetCatalog(tempCatalogName)
	if err != nil {
		fmt.Printf("Error getting catalog: %v\n", err)
		_ = d.DeleteCatalog(adminCatalog)
		os.Exit(1)
	}

	// Upload ISO
	mediaName := "debian-12.12.0-amd64-netinst.iso"
	fmt.Printf("\nUploading ISO as '%s'...\n", mediaName)
	fmt.Println("This may take several minutes...")

	startTime := time.Now()
	media, err := d.UploadMediaImage(catalog, mediaName, "Debian 12.12.0 netinst ISO", isoPath)
	if err != nil {
		fmt.Printf("Error uploading ISO: %v\n", err)
		fmt.Println("Cleaning up catalog...")
		_ = d.DeleteCatalog(adminCatalog)
		os.Exit(1)
	}

	elapsed := time.Since(startTime)
	fmt.Printf("ISO uploaded successfully in %s!\n", elapsed.Round(time.Second))
	fmt.Printf("  Media Name: %s\n", media.Media.Name)
	fmt.Printf("  Media HREF: %s\n", media.Media.HREF)

	// Ask about cleanup
	fmt.Printf("\nTemporary catalog '%s' created. Delete it? [y/N]: ", tempCatalogName)
	var answer string
	fmt.Scanln(&answer)
	if strings.ToLower(answer) == "y" {
		fmt.Println("Deleting catalog...")
		err = d.DeleteCatalog(adminCatalog)
		if err != nil {
			fmt.Printf("Error deleting catalog: %v\n", err)
		} else {
			fmt.Println("Catalog deleted.")
		}
	} else {
		fmt.Printf("Catalog '%s' left intact.\n", tempCatalogName)
	}

	fmt.Println("\nTest complete!")
}

func runTest(cmd *cobra.Command, args []string) {
	org := viper.GetString("VCD_ORG")

	d, err := getDriver()
	if err != nil {
		fmt.Printf("Connection failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Connection successful!")

	// Get the underlying client for catalog operations
	client := d.GetClient()

	// Get org
	fmt.Printf("\nGetting org '%s'...\n", org)
	vcdOrg, err := client.GetOrgByName(org)
	if err != nil {
		fmt.Printf("Error getting org: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Org found: %s (HREF: %s)\n", vcdOrg.Org.Name, vcdOrg.Org.HREF)

	// List available VDCs
	fmt.Println("\nListing available VDCs...")
	vdcs, err := vcdOrg.QueryOrgVdcList()
	if err != nil {
		fmt.Printf("Error listing VDCs: %v\n", err)
	} else {
		fmt.Printf("Found %d VDCs:\n", len(vdcs))
		for _, vdc := range vdcs {
			fmt.Printf("  - %s\n", vdc.Name)
		}
	}

	// List existing catalogs
	fmt.Println("\nListing existing catalogs...")
	catalogs, err := vcdOrg.QueryCatalogList()
	if err != nil {
		fmt.Printf("Error listing catalogs: %v\n", err)
	} else {
		fmt.Printf("Found %d catalogs:\n", len(catalogs))
		for _, cat := range catalogs {
			fmt.Printf("  - %s\n", cat.Name)
		}
	}

	fmt.Println("\nTest complete!")
}

func runListNetworks(cmd *cobra.Command, args []string) {
	vdcName := viper.GetString("VCD_VDC")

	d, err := getDriver()
	if err != nil {
		fmt.Printf("Connection failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Connection successful!")

	vdc, err := d.GetVdc(vdcName)
	if err != nil {
		fmt.Printf("Error getting VDC: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("VDC: %s\n\n", vdc.Vdc.Name)

	// List networks
	fmt.Println("Available networks:")
	if vdc.Vdc.AvailableNetworks != nil {
		for _, availNet := range vdc.Vdc.AvailableNetworks {
			if availNet != nil {
				for _, net := range availNet.Network {
					fmt.Printf("  - %s\n", net.Name)
				}
			}
		}
	}

	// List storage profiles
	fmt.Println("\nStorage profiles:")
	if vdc.Vdc.VdcStorageProfiles != nil {
		for _, sp := range vdc.Vdc.VdcStorageProfiles.VdcStorageProfile {
			fmt.Printf("  - %s\n", sp.Name)
		}
	}
}

func runCreateVM(cmd *cobra.Command, args []string) {
	vdcName := viper.GetString("VCD_VDC")
	catalogName, _ := cmd.Flags().GetString("catalog")
	isoName, _ := cmd.Flags().GetString("iso")
	vmName, _ := cmd.Flags().GetString("vm-name")
	vappName, _ := cmd.Flags().GetString("vapp")
	networkName, _ := cmd.Flags().GetString("network")
	storageProfile, _ := cmd.Flags().GetString("storage-profile")

	if vappName == "" {
		vappName = fmt.Sprintf("packer-vapp-%d", time.Now().Unix())
	}

	d, err := getDriver()
	if err != nil {
		fmt.Printf("Connection failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Connection successful!")

	// Get VDC
	fmt.Printf("\nGetting VDC: %s\n", vdcName)
	vdc, err := d.GetVdc(vdcName)
	if err != nil {
		fmt.Printf("Error getting VDC: %v\n", err)
		os.Exit(1)
	}

	// If no network specified, try to find first one
	if networkName == "" && vdc.Vdc.AvailableNetworks != nil && len(vdc.Vdc.AvailableNetworks) > 0 {
		if vdc.Vdc.AvailableNetworks[0] != nil && len(vdc.Vdc.AvailableNetworks[0].Network) > 0 {
			networkName = vdc.Vdc.AvailableNetworks[0].Network[0].Name
			fmt.Printf("Using first available network: %s\n", networkName)
		}
	}

	// If no storage profile, use first one
	if storageProfile == "" && vdc.Vdc.VdcStorageProfiles != nil && len(vdc.Vdc.VdcStorageProfiles.VdcStorageProfile) > 0 {
		storageProfile = vdc.Vdc.VdcStorageProfiles.VdcStorageProfile[0].Name
		fmt.Printf("Using first storage profile: %s\n", storageProfile)
	}

	// Create vApp
	fmt.Printf("\nCreating vApp: %s\n", vappName)
	vapp, err := d.CreateVApp(vdc, vappName, "Packer test vApp", networkName)
	if err != nil {
		fmt.Printf("Error creating vApp: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("vApp created: %s\n", vapp.VApp.Name)

	// Get storage profile reference
	var storageProfileRef *types.Reference
	if storageProfile != "" {
		sp, err := vdc.FindStorageProfileReference(storageProfile)
		if err != nil {
			fmt.Printf("Error finding storage profile: %v\n", err)
		} else {
			storageProfileRef = &sp
		}
	}

	// Create empty VM
	fmt.Printf("\nCreating VM: %s\n", vmName)
	emptyVmParams := &types.RecomposeVAppParamsForEmptyVm{
		XmlnsVcloud: types.XMLNamespaceVCloud,
		XmlnsOvf:    types.XMLNamespaceOVF,
		CreateItem: &types.CreateItem{
			Name:        vmName,
			Description: "Packer test VM",
			VmSpecSection: &types.VmSpecSection{
				Modified:          boolPtr(true),
				Info:              "Virtual Machine specification",
				OsType:            "debian10_64Guest",
				NumCpus:           intPtr(2),
				NumCoresPerSocket: intPtr(1),
				CpuResourceMhz:    &types.CpuResourceMhz{},
				MemoryResourceMb:  &types.MemoryResourceMb{Configured: 2048},
				DiskSection: &types.DiskSection{
					DiskSettings: []*types.DiskSettings{
						{
							SizeMb:            40960,
							UnitNumber:        0,
							BusNumber:         0,
							AdapterType:       "5", // LSI Logic SAS
							ThinProvisioned:   boolPtr(true),
							StorageProfile:    storageProfileRef,
							OverrideVmDefault: true,
						},
					},
				},
				HardwareVersion: &types.HardwareVersion{Value: "vmx-19"},
				VirtualCpuType:  "VM64",
				Firmware:        "bios",
			},
		},
		AllEULAsAccepted: true,
	}

	// Add network if specified
	if networkName != "" {
		emptyVmParams.CreateItem.NetworkConnectionSection = &types.NetworkConnectionSection{
			PrimaryNetworkConnectionIndex: 0,
			NetworkConnection: []*types.NetworkConnection{
				{
					Network:                 networkName,
					NetworkConnectionIndex:  0,
					IsConnected:             true,
					IPAddressAllocationMode: types.IPAllocationModePool,
					NetworkAdapterType:      "VMXNET3",
				},
			},
		}
	}

	vm, err := vapp.AddEmptyVm(emptyVmParams)
	if err != nil {
		fmt.Printf("Error creating VM: %v\n", err)
		fmt.Println("Cleaning up vApp...")
		vapp.Delete()
		os.Exit(1)
	}
	fmt.Printf("VM created: %s\n", vm.VM.Name)

	// Mount ISO
	fmt.Printf("\nMounting ISO: %s from catalog %s\n", isoName, catalogName)
	org, _ := d.GetOrg()
	task, err := vm.HandleInsertMedia(org, catalogName, isoName)
	if err != nil {
		fmt.Printf("Error mounting ISO: %v\n", err)
	} else {
		err = task.WaitTaskCompletion()
		if err != nil {
			fmt.Printf("Error waiting for ISO mount: %v\n", err)
		} else {
			fmt.Println("ISO mounted successfully!")
		}
	}

	// Power on VM
	fmt.Println("\nPowering on VM...")
	task, err = vm.PowerOn()
	if err != nil {
		fmt.Printf("Error powering on VM: %v\n", err)
	} else {
		err = task.WaitTaskCompletion()
		if err != nil {
			fmt.Printf("Error waiting for power on: %v\n", err)
		} else {
			fmt.Println("VM powered on!")
		}
	}

	// Get VM status
	status, _ := vm.GetStatus()
	fmt.Printf("\nVM Status: %s\n", status)
	fmt.Printf("VM HREF: %s\n", vm.VM.HREF)
	fmt.Printf("vApp HREF: %s\n", vapp.VApp.HREF)

	// Ask about cleanup
	fmt.Printf("\nDelete VM and vApp? [y/N]: ")
	var answer string
	fmt.Scanln(&answer)
	if strings.ToLower(answer) == "y" {
		fmt.Println("Powering off VM...")
		powerOffTask, err := vm.PowerOff()
		if err != nil {
			fmt.Printf("Error powering off VM: %v\n", err)
		} else {
			powerOffTask.WaitTaskCompletion()
		}
		fmt.Println("Deleting vApp...")
		deleteTask, err := vapp.Delete()
		if err != nil {
			fmt.Printf("Error deleting vApp: %v\n", err)
		} else {
			deleteTask.WaitTaskCompletion()
			fmt.Println("vApp deleted.")
		}
	} else {
		fmt.Printf("VM '%s' in vApp '%s' left running.\n", vmName, vappName)
	}

	fmt.Println("\nTest complete!")
}

func boolPtr(b bool) *bool {
	return &b
}

func intPtr(i int) *int {
	return &i
}

func runFullTest(cmd *cobra.Command, args []string) {
	isoPath := args[0]
	vdcName := viper.GetString("VCD_VDC")

	// Check if file exists
	if _, err := os.Stat(isoPath); os.IsNotExist(err) {
		fmt.Printf("Error: ISO file not found: %s\n", isoPath)
		os.Exit(1)
	}

	fmt.Printf("ISO file: %s\n", isoPath)
	fmt.Printf("VDC: %s\n\n", vdcName)

	d, err := getDriver()
	if err != nil {
		fmt.Printf("Connection failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Connection successful!")

	// Get VDC
	vdc, err := d.GetVdc(vdcName)
	if err != nil {
		fmt.Printf("Error getting VDC: %v\n", err)
		os.Exit(1)
	}

	// Get first storage profile from VDC
	var storageProfileRef *types.Reference
	if vdc.Vdc.VdcStorageProfiles != nil && len(vdc.Vdc.VdcStorageProfiles.VdcStorageProfile) > 0 {
		storageProfileRef = vdc.Vdc.VdcStorageProfiles.VdcStorageProfile[0]
		fmt.Printf("Using storage profile: %s\n", storageProfileRef.Name)
	} else {
		fmt.Println("Warning: No storage profiles found in VDC")
	}

	// Create temporary catalog with VDC storage profile
	tempCatalogName := fmt.Sprintf("packer-test-%d", time.Now().Unix())
	fmt.Printf("\nCreating catalog '%s' with VDC storage profile...\n", tempCatalogName)

	adminCatalog, err := d.CreateCatalogWithStorageProfile(tempCatalogName, "Packer test catalog", storageProfileRef)
	if err != nil {
		fmt.Printf("Error creating catalog: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Catalog created: %s\n", adminCatalog.AdminCatalog.Name)

	// Get regular catalog reference for upload
	catalog, err := d.GetCatalog(tempCatalogName)
	if err != nil {
		fmt.Printf("Error getting catalog: %v\n", err)
		_ = d.DeleteCatalog(adminCatalog)
		os.Exit(1)
	}

	// Upload ISO
	mediaName := "debian-netinst.iso"
	fmt.Printf("\nUploading ISO as '%s'...\n", mediaName)
	fmt.Println("This may take several minutes...")

	startTime := time.Now()
	media, err := d.UploadMediaImage(catalog, mediaName, "Debian netinst ISO", isoPath)
	if err != nil {
		fmt.Printf("Error uploading ISO: %v\n", err)
		fmt.Println("Cleaning up catalog...")
		_ = d.DeleteCatalog(adminCatalog)
		os.Exit(1)
	}
	elapsed := time.Since(startTime)
	fmt.Printf("ISO uploaded successfully in %s!\n", elapsed.Round(time.Second))
	fmt.Printf("  Media Name: %s\n", media.Media.Name)

	// Get first network
	networkName := ""
	if vdc.Vdc.AvailableNetworks != nil && len(vdc.Vdc.AvailableNetworks) > 0 {
		if vdc.Vdc.AvailableNetworks[0] != nil && len(vdc.Vdc.AvailableNetworks[0].Network) > 0 {
			networkName = vdc.Vdc.AvailableNetworks[0].Network[0].Name
			fmt.Printf("\nUsing network: %s\n", networkName)
		}
	}

	// Create vApp
	vappName := fmt.Sprintf("packer-vapp-%d", time.Now().Unix())
	fmt.Printf("\nCreating vApp: %s\n", vappName)
	vapp, err := d.CreateVApp(vdc, vappName, "Packer test vApp", networkName)
	if err != nil {
		fmt.Printf("Error creating vApp: %v\n", err)
		fmt.Println("Cleaning up...")
		_ = d.DeleteCatalog(adminCatalog)
		os.Exit(1)
	}
	fmt.Printf("vApp created: %s\n", vapp.VApp.Name)

	// Create empty VM
	vmName := "packer-test-vm"
	fmt.Printf("\nCreating VM: %s\n", vmName)

	emptyVmParams := &types.RecomposeVAppParamsForEmptyVm{
		XmlnsVcloud: types.XMLNamespaceVCloud,
		XmlnsOvf:    types.XMLNamespaceOVF,
		CreateItem: &types.CreateItem{
			Name:        vmName,
			Description: "Packer test VM",
			VmSpecSection: &types.VmSpecSection{
				Modified:          boolPtr(true),
				Info:              "Virtual Machine specification",
				OsType:            "debian10_64Guest",
				NumCpus:           intPtr(2),
				NumCoresPerSocket: intPtr(1),
				CpuResourceMhz:    &types.CpuResourceMhz{},
				MemoryResourceMb:  &types.MemoryResourceMb{Configured: 2048},
				DiskSection: &types.DiskSection{
					DiskSettings: []*types.DiskSettings{
						{
							SizeMb:            40960,
							UnitNumber:        0,
							BusNumber:         0,
							AdapterType:       "5", // LSI Logic SAS
							ThinProvisioned:   boolPtr(true),
							StorageProfile:    storageProfileRef,
							OverrideVmDefault: true,
						},
					},
				},
				HardwareVersion: &types.HardwareVersion{Value: "vmx-19"},
				VirtualCpuType:  "VM64",
				Firmware:        "bios",
			},
		},
		AllEULAsAccepted: true,
	}

	// Add network if specified
	if networkName != "" {
		emptyVmParams.CreateItem.NetworkConnectionSection = &types.NetworkConnectionSection{
			PrimaryNetworkConnectionIndex: 0,
			NetworkConnection: []*types.NetworkConnection{
				{
					Network:                 networkName,
					NetworkConnectionIndex:  0,
					IsConnected:             true,
					IPAddressAllocationMode: types.IPAllocationModePool,
					NetworkAdapterType:      "VMXNET3",
				},
			},
		}
	}

	vm, err := vapp.AddEmptyVm(emptyVmParams)
	if err != nil {
		fmt.Printf("Error creating VM: %v\n", err)
		fmt.Println("Cleaning up...")
		vapp.Delete()
		_ = d.DeleteCatalog(adminCatalog)
		os.Exit(1)
	}
	fmt.Printf("VM created: %s\n", vm.VM.Name)

	// Mount ISO
	fmt.Printf("\nMounting ISO: %s from catalog %s\n", mediaName, tempCatalogName)
	org, _ := d.GetOrg()
	task, err := vm.HandleInsertMedia(org, tempCatalogName, mediaName)
	if err != nil {
		fmt.Printf("Error mounting ISO: %v\n", err)
	} else {
		err = task.WaitTaskCompletion()
		if err != nil {
			fmt.Printf("Error waiting for ISO mount: %v\n", err)
		} else {
			fmt.Println("ISO mounted successfully!")
		}
	}

	// Power on VM
	fmt.Println("\nPowering on VM...")
	task, err = vm.PowerOn()
	if err != nil {
		fmt.Printf("Error powering on VM: %v\n", err)
	} else {
		err = task.WaitTaskCompletion()
		if err != nil {
			fmt.Printf("Error waiting for power on: %v\n", err)
		} else {
			fmt.Println("VM powered on!")
		}
	}

	// Get VM status
	status, _ := vm.GetStatus()
	fmt.Printf("\nVM Status: %s\n", status)
	fmt.Printf("VM HREF: %s\n", vm.VM.HREF)
	fmt.Printf("vApp HREF: %s\n", vapp.VApp.HREF)
	fmt.Printf("Catalog: %s\n", tempCatalogName)

	// Ask about cleanup
	fmt.Printf("\nDelete VM, vApp and catalog? [y/N]: ")
	var answer string
	fmt.Scanln(&answer)
	if strings.ToLower(answer) == "y" {
		fmt.Println("Powering off VM...")
		powerOffTask, err := vm.PowerOff()
		if err != nil {
			fmt.Printf("Note: %v\n", err)
		} else {
			powerOffTask.WaitTaskCompletion()
		}

		fmt.Println("Undeploying vApp...")
		undeployTask, err := vapp.Undeploy()
		if err != nil {
			fmt.Printf("Note: %v\n", err)
		} else {
			undeployTask.WaitTaskCompletion()
		}

		fmt.Println("Deleting vApp...")
		deleteTask, err := vapp.Delete()
		if err != nil {
			fmt.Printf("Error deleting vApp: %v\n", err)
		} else {
			deleteTask.WaitTaskCompletion()
			fmt.Println("vApp deleted.")
		}

		fmt.Println("Deleting catalog...")
		err = d.DeleteCatalog(adminCatalog)
		if err != nil {
			fmt.Printf("Error deleting catalog: %v\n", err)
		} else {
			fmt.Println("Catalog deleted.")
		}
	} else {
		fmt.Printf("Resources left intact. Remember to clean up:\n")
		fmt.Printf("  - vApp: %s\n", vappName)
		fmt.Printf("  - Catalog: %s\n", tempCatalogName)
	}

	fmt.Println("\nFull test complete!")
}

func runConsoleTest(cmd *cobra.Command, args []string) {
	vmHref := args[0]
	text, _ := cmd.Flags().GetString("text")
	pressEnter, _ := cmd.Flags().GetBool("enter")

	d, err := getDriver()
	if err != nil {
		fmt.Printf("Connection failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Connection successful!")

	client := d.GetClient()

	// Get MKS ticket
	fmt.Printf("\nAcquiring MKS ticket for VM: %s\n", vmHref)
	ticket, err := driver.AcquireMksTicketDirect(client, vmHref)
	if err != nil {
		fmt.Printf("Error acquiring MKS ticket: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("MKS Ticket acquired:\n")
	fmt.Printf("  Host: %s\n", ticket.Host)
	fmt.Printf("  Port: %d\n", ticket.Port)
	fmt.Printf("  Ticket: %s...\n", ticket.Ticket[:min(20, len(ticket.Ticket))])
	fmt.Printf("  WebSocket URL: %s\n", ticket.WebSocketURL())

	// Connect to console
	fmt.Println("\nConnecting to VM console...")
	wmks := driver.NewWMKSClient(ticket, driver.WithInsecure(true))
	err = wmks.Connect()
	if err != nil {
		fmt.Printf("Error connecting to console: %v\n", err)
		os.Exit(1)
	}
	defer wmks.Close()
	fmt.Println("Connected to VM console!")

	// Wait a moment for connection to stabilize
	time.Sleep(200 * time.Millisecond)

	// Send text
	fmt.Printf("\nSending text: %q\n", text)
	err = wmks.SendString(text)
	if err != nil {
		fmt.Printf("Error sending text: %v\n", err)
		os.Exit(1)
	}

	if pressEnter {
		fmt.Println("Pressing Enter...")
		err = wmks.SendSpecialKey("ENTER")
		if err != nil {
			fmt.Printf("Error pressing Enter: %v\n", err)
			os.Exit(1)
		}
	}

	fmt.Println("\nConsole test complete!")
}
