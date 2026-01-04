package driver

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/vmware/go-vcloud-director/v3/govcd"
	"github.com/vmware/go-vcloud-director/v3/types/v56"
)

const vcdAPIVersion = "38.1"

// Driver defines the interface for VCD operations
type Driver interface {
	// VM operations
	NewVM(ref *govcd.VM) VirtualMachine
	FindVM(vdcName, vappName, vmName string) (VirtualMachine, error)

	// Org operations
	GetOrg() (*govcd.Org, error)
	GetAdminOrg() (*govcd.AdminOrg, error)

	// VDC operations
	GetVdc(name string) (*govcd.Vdc, error)

	// vApp operations
	GetVApp(vdcName, vappName string) (*govcd.VApp, error)
	CreateVApp(vdc *govcd.Vdc, name, description, networkName string) (*govcd.VApp, error)

	// Catalog operations
	GetCatalog(name string) (*govcd.Catalog, error)
	CreateCatalog(name, description string) (*govcd.AdminCatalog, error)
	CreateCatalogWithStorageProfile(name, description string, storageProfileRef *types.Reference) (*govcd.AdminCatalog, error)
	DeleteCatalog(catalog *govcd.AdminCatalog) error
	UploadMediaImage(catalog *govcd.Catalog, name, description, filePath string) (*govcd.Media, error)

	// Lifecycle
	Cleanup() error
	GetClient() *govcd.VCDClient
}

type VCDDriver struct {
	client  *govcd.VCDClient
	orgName string
}

func NewVCDDriver(client *govcd.VCDClient, orgName string) Driver {
	return &VCDDriver{
		client:  client,
		orgName: orgName,
	}
}

type ConnectConfig struct {
	Host               string
	Org                string
	Username           string
	Password           string
	Token              string
	InsecureConnection bool
}

func NewDriver(config *ConnectConfig) (Driver, error) {
	apiURL, err := url.Parse(fmt.Sprintf("https://%s/api", config.Host))
	if err != nil {
		return nil, err
	}

	govcdClient, err := newClient(*apiURL, config.Org, config.Username, config.Password, config.Token, config.InsecureConnection)
	if err != nil {
		return nil, err
	}

	driver := &VCDDriver{
		client:  govcdClient,
		orgName: config.Org,
	}

	return driver, nil
}

func (d *VCDDriver) Cleanup() error {
	if d.client != nil {
		return d.client.Disconnect()
	}
	return nil
}

func (d *VCDDriver) GetClient() *govcd.VCDClient {
	return d.client
}

// --- VM Operations ---

func (d *VCDDriver) NewVM(ref *govcd.VM) VirtualMachine {
	return &VirtualMachineDriver{
		vm:     ref,
		driver: d,
	}
}

func (d *VCDDriver) FindVM(vdcName, vappName, vmName string) (VirtualMachine, error) {
	vdc, err := d.GetVdc(vdcName)
	if err != nil {
		return nil, fmt.Errorf("error getting VDC %s: %w", vdcName, err)
	}

	vapp, err := vdc.GetVAppByName(vappName, true)
	if err != nil {
		return nil, fmt.Errorf("error getting vApp %s: %w", vappName, err)
	}

	vm, err := vapp.GetVMByName(vmName, true)
	if err != nil {
		return nil, fmt.Errorf("error getting VM %s: %w", vmName, err)
	}

	return d.NewVM(vm), nil
}

// --- Org Operations ---

func (d *VCDDriver) GetOrg() (*govcd.Org, error) {
	org, err := d.client.GetOrgByName(d.orgName)
	if err != nil {
		return nil, fmt.Errorf("error getting org %s: %w", d.orgName, err)
	}
	return org, nil
}

func (d *VCDDriver) GetAdminOrg() (*govcd.AdminOrg, error) {
	adminOrg, err := d.client.GetAdminOrgByName(d.orgName)
	if err != nil {
		return nil, fmt.Errorf("error getting admin org %s: %w", d.orgName, err)
	}
	return adminOrg, nil
}

// --- VDC Operations ---

func (d *VCDDriver) GetVdc(name string) (*govcd.Vdc, error) {
	org, err := d.GetOrg()
	if err != nil {
		return nil, err
	}

	vdc, err := org.GetVDCByName(name, true)
	if err != nil {
		return nil, fmt.Errorf("error getting VDC %s: %w", name, err)
	}
	return vdc, nil
}

// --- vApp Operations ---

func (d *VCDDriver) GetVApp(vdcName, vappName string) (*govcd.VApp, error) {
	vdc, err := d.GetVdc(vdcName)
	if err != nil {
		return nil, err
	}

	vapp, err := vdc.GetVAppByName(vappName, true)
	if err != nil {
		return nil, fmt.Errorf("error getting vApp %s: %w", vappName, err)
	}
	return vapp, nil
}

func (d *VCDDriver) CreateVApp(vdc *govcd.Vdc, name, description, networkName string) (*govcd.VApp, error) {
	// Create an empty vApp
	vapp, err := vdc.CreateRawVApp(name, description)
	if err != nil {
		return nil, fmt.Errorf("error creating vApp %s: %w", name, err)
	}

	// Add network to vApp if specified
	if networkName != "" {
		network, err := vdc.GetOrgVdcNetworkByName(networkName, true)
		if err != nil {
			// Try to clean up the vApp we just created
			_, _ = vapp.Delete()
			return nil, fmt.Errorf("error getting network %s: %w", networkName, err)
		}

		_, err = vapp.AddOrgNetwork(&govcd.VappNetworkSettings{}, network.OrgVDCNetwork, false)
		if err != nil {
			// Try to clean up the vApp we just created
			_, _ = vapp.Delete()
			return nil, fmt.Errorf("error adding network to vApp: %w", err)
		}
	}

	return vapp, nil
}

// --- Catalog Operations ---

func (d *VCDDriver) GetCatalog(name string) (*govcd.Catalog, error) {
	org, err := d.GetOrg()
	if err != nil {
		return nil, err
	}

	catalog, err := org.GetCatalogByName(name, true)
	if err != nil {
		return nil, fmt.Errorf("error getting catalog %s: %w", name, err)
	}
	return catalog, nil
}

func (d *VCDDriver) CreateCatalog(name, description string) (*govcd.AdminCatalog, error) {
	adminOrg, err := d.GetAdminOrg()
	if err != nil {
		return nil, err
	}

	catalog, err := adminOrg.CreateCatalog(name, description)
	if err != nil {
		return nil, fmt.Errorf("error creating catalog %s: %w", name, err)
	}
	return &catalog, nil
}

func (d *VCDDriver) CreateCatalogWithStorageProfile(name, description string, storageProfileRef *types.Reference) (*govcd.AdminCatalog, error) {
	adminOrg, err := d.GetAdminOrg()
	if err != nil {
		return nil, err
	}

	var storageProfiles *types.CatalogStorageProfiles
	if storageProfileRef != nil {
		storageProfiles = &types.CatalogStorageProfiles{
			VdcStorageProfile: []*types.Reference{storageProfileRef},
		}
	}

	catalog, err := adminOrg.CreateCatalogWithStorageProfile(name, description, storageProfiles)
	if err != nil {
		return nil, fmt.Errorf("error creating catalog %s with storage profile: %w", name, err)
	}
	return catalog, nil
}

func (d *VCDDriver) DeleteCatalog(catalog *govcd.AdminCatalog) error {
	err := catalog.Delete(true, true)
	if err != nil {
		return fmt.Errorf("error deleting catalog: %w", err)
	}
	return nil
}

func (d *VCDDriver) UploadMediaImage(catalog *govcd.Catalog, name, description, filePath string) (*govcd.Media, error) {
	// Upload with 10MB chunk size
	const uploadPieceSize = 10 * 1024 * 1024

	uploadTask, err := catalog.UploadMediaImage(name, description, filePath, uploadPieceSize)
	if err != nil {
		return nil, fmt.Errorf("error starting media upload: %w", err)
	}

	// Wait for upload to complete
	err = uploadTask.ShowUploadProgress()
	if err != nil {
		return nil, fmt.Errorf("error during media upload: %w", err)
	}

	// Get the uploaded media
	media, err := catalog.GetMediaByName(name, true)
	if err != nil {
		return nil, fmt.Errorf("error getting uploaded media %s: %w", name, err)
	}

	return media, nil
}

// --- Internal helpers ---

func newClient(apiURL url.URL, org string, username string, password string, token string, insecure bool) (*govcd.VCDClient, error) {
	client := &govcd.VCDClient{
		Client: govcd.Client{
			VCDHREF:    apiURL,
			APIVersion: vcdAPIVersion,
			Http: http.Client{
				Transport: &http.Transport{
					TLSClientConfig: &tls.Config{
						InsecureSkipVerify: insecure,
					},
					Proxy:               http.ProxyFromEnvironment,
					TLSHandshakeTimeout: 120 * time.Second,
				},
				Timeout: 600 * time.Second,
			},
			MaxRetryTimeout: 60,
		},
	}

	if token != "" {
		err := client.SetToken(org, govcd.ApiTokenHeader, token)
		if err != nil {
			return nil, fmt.Errorf("unable to authenticate to Org %q: %w", org, err)
		}
	} else {
		err := client.Authenticate(username, password, org)
		if err != nil {
			return nil, fmt.Errorf("unable to authenticate to Org %q: %w", org, err)
		}
	}

	return client, nil
}
