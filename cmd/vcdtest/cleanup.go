package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var cleanupCmd = &cobra.Command{
	Use:   "cleanup",
	Short: "Delete orphaned vApps and catalogs from failed Packer builds",
	Run:   runCleanup,
}

func init() {
	cleanupCmd.Flags().StringSlice("vapp", nil, "vApp name(s) to delete")
	cleanupCmd.Flags().StringSlice("catalog", nil, "Catalog name(s) to delete")
}

func runCleanup(cmd *cobra.Command, args []string) {
	vappNames, _ := cmd.Flags().GetStringSlice("vapp")
	catalogNames, _ := cmd.Flags().GetStringSlice("catalog")

	if len(vappNames) == 0 && len(catalogNames) == 0 {
		fmt.Println("Error: specify at least one --vapp or --catalog to delete")
		fmt.Println("Example: vcdtest cleanup --vapp packer-123 --catalog packer-456")
		os.Exit(1)
	}

	vdcName := getEnv("VCD_VDC", "PKR_VAR_vcd_vdc")
	if vdcName == "" && len(vappNames) > 0 {
		fmt.Println("Error: VCD_VDC (or PKR_VAR_vcd_vdc) environment variable is required for vApp cleanup")
		os.Exit(1)
	}

	d, err := getDriver()
	if err != nil {
		fmt.Printf("Connection failed: %v\n", err)
		os.Exit(1)
	}
	defer d.Cleanup()
	fmt.Println("Connection successful!\n")

	hasErrors := false

	// Delete vApps
	for _, vappName := range vappNames {
		fmt.Printf("=== Deleting vApp: %s ===\n", vappName)

		vdc, err := d.GetVdc(vdcName)
		if err != nil {
			fmt.Printf("  Error getting VDC %s: %v\n", vdcName, err)
			hasErrors = true
			continue
		}

		vapp, err := vdc.GetVAppByName(vappName, true)
		if err != nil {
			fmt.Printf("  vApp not found: %v\n", err)
			hasErrors = true
			continue
		}

		// Refresh to get current state
		if err := vapp.Refresh(); err != nil {
			fmt.Printf("  Error refreshing vApp state: %v\n", err)
			hasErrors = true
			continue
		}

		// Check status and power off if needed
		status, err := vapp.GetStatus()
		if err != nil {
			fmt.Printf("  Error getting vApp status: %v\n", err)
		} else {
			fmt.Printf("  Current status: %s\n", status)
		}

		if status != "POWERED_OFF" && status != "RESOLVED" {
			fmt.Printf("  Powering off vApp...\n")
			task, err := vapp.PowerOff()
			if err != nil {
				fmt.Printf("  Note: power off returned: %v\n", err)
			} else {
				if err := task.WaitTaskCompletion(); err != nil {
					fmt.Printf("  Error waiting for power off: %v\n", err)
				} else {
					fmt.Printf("  Powered off.\n")
				}
			}
		}

		// Undeploy if needed
		if err := vapp.Refresh(); err == nil {
			status, _ := vapp.GetStatus()
			if status != "RESOLVED" {
				fmt.Printf("  Undeploying vApp...\n")
				task, err := vapp.Undeploy()
				if err != nil {
					fmt.Printf("  Note: undeploy returned: %v\n", err)
				} else {
					if err := task.WaitTaskCompletion(); err != nil {
						fmt.Printf("  Error waiting for undeploy: %v\n", err)
					} else {
						fmt.Printf("  Undeployed.\n")
					}
				}
			}
		}

		// Delete
		fmt.Printf("  Deleting vApp...\n")
		task, err := vapp.Delete()
		if err != nil {
			fmt.Printf("  Error deleting vApp: %v\n", err)
			hasErrors = true
			continue
		}
		if err := task.WaitTaskCompletion(); err != nil {
			fmt.Printf("  Error waiting for vApp deletion: %v\n", err)
			hasErrors = true
		} else {
			fmt.Printf("  vApp '%s' deleted successfully!\n", vappName)
		}
		fmt.Println()
	}

	// Delete catalogs
	for _, catalogName := range catalogNames {
		fmt.Printf("=== Deleting catalog: %s ===\n", catalogName)

		adminOrg, err := d.GetAdminOrg()
		if err != nil {
			fmt.Printf("  Error getting admin org: %v\n", err)
			hasErrors = true
			continue
		}

		adminCatalog, err := adminOrg.GetAdminCatalogByName(catalogName, true)
		if err != nil {
			fmt.Printf("  Catalog not found: %v\n", err)
			hasErrors = true
			continue
		}

		fmt.Printf("  Deleting catalog (force=true, recursive=true)...\n")
		err = d.DeleteCatalog(adminCatalog)
		if err != nil {
			fmt.Printf("  Error deleting catalog: %v\n", err)
			hasErrors = true
		} else {
			fmt.Printf("  Catalog '%s' deleted successfully!\n", catalogName)
		}
		fmt.Println()
	}

	if hasErrors {
		fmt.Println("Cleanup completed with some errors.")
		os.Exit(1)
	}
	fmt.Println("Cleanup completed successfully!")
}
