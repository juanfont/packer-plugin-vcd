package vcd

import "github.com/hashicorp/packer-plugin-sdk/common"

type Config struct {
	common.PackerConfig `mapstructure:",squash"`
	// The fully qualified domain name or IP address of the vCloud Director endpoint.
	Host string `mapstructure:"host" required:"true"`
	// The username to use to authenticate to the vCloud Director endpoint.
	Username string `mapstructure:"username" required:"true"`
	// The password to use to authenticate to the vCloud Director endpoint.
	Password string `mapstructure:"password" required:"true"`
	// The token to use to authenticate to the vCloud Director endpoint (if not provided, username and password will be used)
	Token string `mapstructure:"token"`
	// Skip the verification of the server certificate. Defaults to `false`.
	Insecure bool `mapstructure:"insecure"`
	// The name of the virtual datacenter to use.
	// Required when the vCloud Director instance endpoint has more than one virtual datacenter.
	VirtualDatacenter string `mapstructure:"virtual_datacenter"`
}
