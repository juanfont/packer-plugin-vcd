package driver

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/vmware/go-vcloud-director/v3/govcd"
)

const vcdAPIVersion = "38.1"

type Driver interface {
	NewVM(ref *govcd.VM) VirtualMachine
	FindVM(name string) (VirtualMachine, error)
	Cleanup() error
}

type VCDDriver struct {
	client *govcd.VCDClient
}

func NewVCDDriver(client *govcd.VCDClient) Driver {
	return &VCDDriver{
		client: client,
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
		client: govcdClient,
	}

	return driver, nil
}

func (d *VCDDriver) Cleanup() error {
	return nil
}

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
			return nil, fmt.Errorf("unable to authenticate to Org \"%s\": %s", org, err)
		}
	} else {
		err := client.Authenticate(username, password, org)
		if err != nil {
			return nil, fmt.Errorf("unable to authenticate to Org \"%s\": %s", org, err)
		}
	}

	return client, nil
}
