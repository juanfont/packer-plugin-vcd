package main

import (
	"fmt"
	"log"
	"net/url"
	"os"

	"github.com/vmware/go-vcloud-director/v3/govcd"
)

func main() {
	host := os.Getenv("VCD_HOST")
	username := os.Getenv("VCD_USERNAME")
	password := os.Getenv("VCD_PASSWORD")
	org := os.Getenv("VCD_ORG")
	vdcName := os.Getenv("VCD_VDC")
	vappName := os.Getenv("VAPP_NAME")
	catalogName := os.Getenv("CATALOG_NAME")

	vcdURL, _ := url.Parse(fmt.Sprintf("https://%s/api", host))
	vcdClient := govcd.NewVCDClient(*vcdURL, true)
	err := vcdClient.Authenticate(username, password, org)
	if err != nil {
		log.Fatalf("Auth failed: %v", err)
	}

	adminOrg, err := vcdClient.GetAdminOrgByName(org)
	if err != nil {
		log.Fatalf("Get org failed: %v", err)
	}

	// Delete vApp
	vdc, err := adminOrg.GetVDCByName(vdcName, false)
	if err != nil {
		log.Fatalf("Get VDC failed: %v", err)
	}

	if vappName != "" {
		vapp, err := vdc.GetVAppByName(vappName, false)
		if err == nil {
			fmt.Printf("Powering off and deleting vApp %s...\n", vappName)
			task, err := vapp.PowerOff()
			if err == nil {
				task.WaitTaskCompletion()
			}
			task, err = vapp.Undeploy()
			if err == nil {
				task.WaitTaskCompletion()
			}
			task, err = vapp.Delete()
			if err != nil {
				log.Printf("Delete vApp error: %v", err)
			} else {
				task.WaitTaskCompletion()
				fmt.Println("vApp deleted")
			}
		} else {
			fmt.Printf("vApp not found: %v\n", err)
		}
	}

	// Delete catalog
	if catalogName != "" {
		catalog, err := adminOrg.GetAdminCatalogByName(catalogName, false)
		if err == nil {
			fmt.Printf("Deleting catalog %s...\n", catalogName)
			err = catalog.Delete(true, true)
			if err != nil {
				log.Printf("Delete catalog error: %v", err)
			} else {
				fmt.Println("Catalog deleted")
			}
		} else {
			fmt.Printf("Catalog not found: %v\n", err)
		}
	}

	fmt.Println("Cleanup complete")
}
