package driver

import (
	"crypto/tls"
	"encoding/xml"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/vmware/go-vcloud-director/v3/govcd"
	"github.com/vmware/go-vcloud-director/v3/types/v56"
)

const vcdAPIVersion = "38.1"

// NetworkInfo contains information about a network's IP configuration
type NetworkInfo struct {
	Gateway     string
	Netmask     string
	DNS1        string
	DNS2        string
	AvailableIP string // First available IP from pool
}

// TrustedPlatformModuleEdit is used to enable/disable TPM on a VM
// API: POST {vm}/action/editTrustedPlatformModule
// Content-Type: application/vnd.vmware.vcloud.TpmSection+xml
type TrustedPlatformModuleEdit struct {
	XMLName    xml.Name `xml:"root:TrustedPlatformModule"`
	Xmlns      string   `xml:"xmlns:root,attr"`
	TpmPresent bool     `xml:"root:TpmPresent"`
}

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

	// Network operations
	FindAvailableIP(vdc *govcd.Vdc, networkName string) (*NetworkInfo, error)
	FindAvailableIPExcluding(vdc *govcd.Vdc, networkName string, excludeIPs []string) (*NetworkInfo, error)
	GetNetworkInfo(vdc *govcd.Vdc, networkName string) (*NetworkInfo, error)

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

	// Wait for vApp to be ready (status RESOLVED = 8)
	// VCD operations are asynchronous, we need to poll until the vApp is ready
	timeout := time.After(5 * time.Minute)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			_, _ = vapp.Delete()
			return nil, fmt.Errorf("timeout waiting for vApp %s to be ready", name)
		case <-ticker.C:
			err := vapp.Refresh()
			if err != nil {
				continue // Retry on refresh errors
			}
			// Status 8 = RESOLVED (ready), Status 4 = POWERED_OFF is also acceptable
			status, err := vapp.GetStatus()
			if err != nil {
				continue
			}
			if status == "RESOLVED" || status == "POWERED_OFF" {
				goto vappReady
			}
		}
	}
vappReady:

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

// --- Network Operations ---

func (d *VCDDriver) FindAvailableIP(vdc *govcd.Vdc, networkName string) (*NetworkInfo, error) {
	return d.FindAvailableIPExcluding(vdc, networkName, nil)
}

func (d *VCDDriver) FindAvailableIPExcluding(vdc *govcd.Vdc, networkName string, excludeIPs []string) (*NetworkInfo, error) {
	network, err := vdc.GetOrgVdcNetworkByName(networkName, true)
	if err != nil {
		return nil, fmt.Errorf("error getting network %s: %w", networkName, err)
	}

	cfg := network.OrgVDCNetwork.Configuration
	if cfg == nil || cfg.IPScopes == nil || len(cfg.IPScopes.IPScope) == 0 {
		return nil, fmt.Errorf("network %s has no IP configuration", networkName)
	}

	ipScope := cfg.IPScopes.IPScope[0]

	// Build set of allocated IPs from VCD's IP tracking
	allocated := make(map[string]bool)
	if ipScope.AllocatedIPAddresses != nil {
		for _, ip := range ipScope.AllocatedIPAddresses.IPAddress {
			allocated[ip] = true
		}
	}
	// Also exclude gateway
	allocated[ipScope.Gateway] = true

	// Exclude IPs that we know are in conflict (from retry logic)
	for _, ip := range excludeIPs {
		allocated[ip] = true
	}

	// IMPORTANT: Also check IPs actually in use by VMs in this VDC
	// VCD's AllocatedIPAddresses doesn't track MANUAL allocations
	usedIPs, err := d.getUsedIPsInVDC(vdc, networkName)
	if err != nil {
		// Log warning but continue - we'll still have the allocated list
		// The build might fail at power-on if there's a conflict
	} else {
		for _, ip := range usedIPs {
			allocated[ip] = true
		}
	}

	// Find first available IP in ranges
	var availableIP string
	if ipScope.IPRanges != nil {
		for _, r := range ipScope.IPRanges.IPRange {
			ip := findFirstAvailableIP(r.StartAddress, r.EndAddress, allocated)
			if ip != "" {
				availableIP = ip
				break
			}
		}
	}

	if availableIP == "" {
		return nil, fmt.Errorf("no available IPs in network %s", networkName)
	}

	return &NetworkInfo{
		Gateway:     ipScope.Gateway,
		Netmask:     ipScope.Netmask,
		DNS1:        ipScope.DNS1,
		DNS2:        ipScope.DNS2,
		AvailableIP: availableIP,
	}, nil
}

// GetNetworkInfo returns network configuration (gateway, netmask, DNS) without finding an available IP
func (d *VCDDriver) GetNetworkInfo(vdc *govcd.Vdc, networkName string) (*NetworkInfo, error) {
	network, err := vdc.GetOrgVdcNetworkByName(networkName, true)
	if err != nil {
		return nil, fmt.Errorf("error getting network %s: %w", networkName, err)
	}

	cfg := network.OrgVDCNetwork.Configuration
	if cfg == nil || cfg.IPScopes == nil || len(cfg.IPScopes.IPScope) == 0 {
		return nil, fmt.Errorf("network %s has no IP configuration", networkName)
	}

	ipScope := cfg.IPScopes.IPScope[0]

	return &NetworkInfo{
		Gateway: ipScope.Gateway,
		Netmask: ipScope.Netmask,
		DNS1:    ipScope.DNS1,
		DNS2:    ipScope.DNS2,
	}, nil
}

// getUsedIPsInVDC queries all VMs in the VDC to find IPs actually in use on a network
func (d *VCDDriver) getUsedIPsInVDC(vdc *govcd.Vdc, networkName string) ([]string, error) {
	var usedIPs []string

	// Get all vApps in the VDC
	vappRefs := vdc.GetVappList()

	for _, vappRef := range vappRefs {
		vapp, err := vdc.GetVAppByName(vappRef.Name, true)
		if err != nil {
			continue // Skip vApps we can't access
		}

		// Check each VM in the vApp
		if vapp.VApp.Children == nil {
			continue
		}

		for _, vmRef := range vapp.VApp.Children.VM {
			// Get network connection section
			if vmRef.NetworkConnectionSection == nil {
				continue
			}

			for _, conn := range vmRef.NetworkConnectionSection.NetworkConnection {
				// Collect ALL IPs from ALL networks - VCD validates across all networks
				// Network isolation happens at a different layer
				if conn.IPAddress != "" {
					usedIPs = append(usedIPs, conn.IPAddress)
				}
			}
		}
	}

	return usedIPs, nil
}

// findFirstAvailableIP finds the first IP in the range that is not in the allocated set
func findFirstAvailableIP(start, end string, allocated map[string]bool) string {
	startIP := net.ParseIP(start).To4()
	endIP := net.ParseIP(end).To4()
	if startIP == nil || endIP == nil {
		return ""
	}

	// Make a copy of startIP to iterate
	ip := make(net.IP, len(startIP))
	copy(ip, startIP)

	for {
		ipStr := ip.String()
		if !allocated[ipStr] {
			return ipStr
		}

		// Check if we've reached the end
		if ip.Equal(endIP) {
			break
		}

		// Increment IP
		incrementIP(ip)
	}

	return ""
}

// incrementIP increments an IP address by 1
func incrementIP(ip net.IP) {
	for i := len(ip) - 1; i >= 0; i-- {
		ip[i]++
		if ip[i] != 0 {
			break
		}
	}
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
