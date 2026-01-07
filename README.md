# Packer Plugin for VMware Cloud Director

The Packer Plugin for VMware Cloud Director is a plugin that can be used to create virtual machine
images on [VMware Cloud Director][vmware-vcd] (VCD).

The plugin includes one builder:

- `vcd-iso` - This builder creates a virtual machine, uploads an ISO to a VCD catalog, installs an
  operating system using boot commands, provisions software within the operating system, and then
  exports the virtual machine as a vApp template. This is best for those who want to create images
  from scratch using ISO files.

## Features

- **ISO-based VM creation** - Upload ISOs to VCD catalogs and create VMs from scratch
- **Boot command support** - Send keystrokes to VM console via WebMKS protocol for automated OS installation
- **HTTP server** - Serve kickstart/preseed files during installation
- **SSH/WinRM communicator** - Connect to VMs for provisioning (Linux and Windows)
- **Export to catalog** - Export finished VMs as vApp templates

## Requirements

- [Packer][packer-install] >= 1.10.0
- VMware Cloud Director 10.4+ (API version 38.0+)

> [!NOTE]
> The plugin has been tested with VMware Cloud Director 10.6.

## Usage

For examples on how to use this plugin with Packer refer to the [example](example/) directory of
the repository.

## Installation

### Using Pre-built Releases

#### Automatic Installation

Packer v1.7.0 and later supports the `packer init` command which enables the automatic installation
of Packer plugins. For more information, see the [Packer documentation][docs-packer-init].

To install this plugin, copy and paste this code (HCL2) into your Packer configuration and run
`packer init`.

```hcl
packer {
  required_version = ">= 1.7.0"
  required_plugins {
    vcd = {
      version = ">= 0.0.1"
      source  = "github.com/juanfont/vcd"
    }
  }
}
```

#### Manual Installation

You can download the plugin from the GitHub [releases][releases-vcd-plugin]. Once you have
downloaded the latest release archive for your target operating system and architecture, extract the
release archive to retrieve the plugin binary file for your platform.

To install the downloaded plugin, please follow the Packer documentation on
[installing a plugin][docs-packer-plugin-install].

### From Source

If you prefer to build the plugin from sources, clone the GitHub repository locally and run the
command `go build` from the repository root directory. Upon successful compilation, a
`packer-plugin-vcd` plugin binary file can be found in the root directory.

```bash
git clone https://github.com/juanfont/packer-plugin-vcd.git
cd packer-plugin-vcd
go build
```

To install the compiled plugin, please follow the Packer documentation on
[installing a plugin][docs-packer-plugin-install].

## Configuration

For more information on how to configure the plugin, please see the plugin documentation.

- `vcd-iso` [builder documentation][docs-vcd-iso]

## Network Considerations

For ISO-based builds with preseed/kickstart, the VM needs network connectivity to fetch the preseed
file from the Packer HTTP server during OS installation.

### IP Allocation Modes

VCD supports several IP allocation modes. This plugin supports:

| Mode | Description | Use Case |
|------|-------------|----------|
| `DHCP` | VM gets IP from DHCP server | Networks with DHCP enabled |
| `MANUAL` | Static IP assignment | Networks without DHCP (IP pools) |

> **Note:** The `POOL` mode in VCD allocates IPs from VCD's internal pool but doesn't configure them
> inside the guest OS. For automated installations, use `DHCP` or `MANUAL` with `auto_discover_ip`.

### DHCP Networks

If your VCD network has DHCP enabled:

```hcl
network            = "my-network"
ip_allocation_mode = "DHCP"

# Debian preseed example
boot_command = [
  "auto url=http://{{ .HTTPIP }}:{{ .HTTPPort }}/preseed.cfg<enter>"
]
```

### Static IP Networks (Manual)

If your network uses IP pools without DHCP, you have two options:

#### Option 1: Auto-discover IP (Recommended)

Enable `auto_discover_ip` to automatically find an available IP from the network's pool:

```hcl
network            = "my-network"
ip_allocation_mode = "MANUAL"
auto_discover_ip   = true

# Debian preseed example (kickstart for RHEL/CentOS, autoinstall for Ubuntu)
boot_command = [
  "<esc>auto ",
  "netcfg/disable_autoconfig=true ",
  "netcfg/get_ipaddress={{ .VMIP }} ",
  "netcfg/get_netmask={{ .Netmask }} ",
  "netcfg/get_gateway={{ .Gateway }} ",
  "netcfg/get_nameservers={{ .DNS }} ",
  "preseed/url=http://{{ .HTTPIP }}:{{ .HTTPPort }}/preseed.cfg<enter>"
]
```

This queries the network configuration and makes these template variables available:
- `{{ .VMIP }}` - The discovered available IP address
- `{{ .Gateway }}` - Network gateway
- `{{ .Netmask }}` - Network mask
- `{{ .DNS }}` - DNS server

You can override gateway and DNS if needed:

```hcl
auto_discover_ip = true
vm_gateway       = "10.0.0.1"    # Override discovered gateway
vm_dns           = "8.8.8.8"     # Override discovered DNS
```

#### Option 2: Specify IP manually

If you prefer to specify the IP address manually, use variables for maintainability:

```hcl
variable "vm_ip" {
  type    = string
  default = "10.0.0.100"
}

variable "vm_netmask" {
  type    = string
  default = "255.255.255.0"
}

variable "vm_gateway" {
  type    = string
  default = "10.0.0.1"
}

variable "vm_dns" {
  type    = string
  default = "10.0.0.1"
}

source "vcd-iso" "example" {
  # ...
  network            = "my-network"
  ip_allocation_mode = "MANUAL"
  vm_ip              = var.vm_ip

  # Debian preseed example (other distros: kickstart for RHEL/CentOS, autoinstall for Ubuntu)
  boot_command = [
    "<esc>auto ",
    "netcfg/disable_autoconfig=true ",
    "netcfg/get_ipaddress=${var.vm_ip} ",
    "netcfg/get_netmask=${var.vm_netmask} ",
    "netcfg/get_gateway=${var.vm_gateway} ",
    "netcfg/get_nameservers=${var.vm_dns} ",
    "preseed/url=http://{{ .HTTPIP }}:{{ .HTTPPort }}/preseed.cfg<enter>"
  ]
}
```

> **Note:** The boot command syntax above uses Debian preseed. Other distributions use different
> automated installation methods: kickstart for RHEL/CentOS/Fedora, autoinstall for Ubuntu 20.04+.

## Contributing

If you discover a bug or would like to suggest a feature or an enhancement, please use the GitHub
[issues][issues].

## GenAI Disclaimer

I have used Claude Code for this project. I have been working with VMware Cloud Director and govcd ([docker-machine-driver-vcd][docker-machine-driver-vcd], [fleeting-plugin-vcd][fleeting-plugin-vcd]) for years now, but this project was that expected bigger and it had a major showstopper: VCD does not have an API call to "press keys" and send them to the VM, so in order to type the boot command it was necessary to reverse engineering the WebSockets Web Console and "type" them in the console. Claude helped A LOT.

## License

BSD-3-Clause

[vmware-vcd]: https://www.vmware.com/products/cloud-director.html
[packer-install]: https://developer.hashicorp.com/packer/install
[docs-packer-init]: https://developer.hashicorp.com/packer/docs/commands/init
[docs-packer-plugin-install]: https://developer.hashicorp.com/packer/docs/plugins/install-plugins
[docs-vcd-iso]: https://github.com/juanfont/packer-plugin-vcd/blob/main/docs/builders/vcd-iso.mdx
[releases-vcd-plugin]: https://github.com/juanfont/packer-plugin-vcd/releases
[issues]: https://github.com/juanfont/packer-plugin-vcd/issues
[docker-machine-driver-vcd]: https://github.com/juanfont/docker-machine-driver-vcd
[fleeting-plugin-vcd]: https://github.com/juanfont/fleeting-plugin-vcd
