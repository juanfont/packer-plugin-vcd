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
- **CD Content injection** - Add files directly to the ISO (workaround for VCD single media slot limitation)
- **SSH/WinRM communicator** - Connect to VMs for provisioning (Linux and Windows)
- **EFI and TPM support** - Create UEFI-based VMs with virtual TPM (required for Windows 11)
- **Export to catalog** - Export finished VMs as vApp templates

## VCD Limitations and Workarounds

### Single Media Slot

VMware Cloud Director only supports **one CD-ROM media attached at a time** per VM. This differs from
other virtualization platforms like VMware vSphere or QEMU where you could mount multiple ISO images.

This limitation affects Packer workflows that need to provide additional files (kickstart, preseed,
autounattend.xml) alongside the OS installer ISO.

### The cd_content Solution

This plugin provides the `cd_content` feature as a workaround. Instead of mounting a separate ISO,
`cd_content` **modifies the installer ISO** to include your additional files directly:

```hcl
source "vcd-iso" "example" {
  iso_url      = "https://example.com/debian-12.iso"
  iso_checksum = "sha256:..."

  # Files are injected directly into the ISO
  cd_content = {
    "preseed.cfg"          = file("${path.root}/http/preseed.cfg")
    "scripts/post-install" = file("${path.root}/scripts/post-install.sh")
  }

  # ...
}
```

The modified ISO:
- Contains all original files from the source ISO
- Includes your additional files at the root level
- Maintains bootability (isolinux/grub boot records are preserved)
- Is uploaded to VCD and mounted as the boot media

> **Note:** For Linux ISOs (ISO9660), the plugin uses native Go for ISO manipulation. For Windows ISOs
> (UDF filesystem), external tools are required: `p7zip-full` and `genisoimage`. Files are accessible
> from the mounted CD-ROM inside the VM (e.g., `/cdrom/preseed.cfg` during Debian installation).

## Requirements

- [Packer][packer-install] >= 1.10.0
- VMware Cloud Director 10.4+ (API version 38.0+)
- For Windows ISO modification (cd_content): `p7zip-full` and `genisoimage`
  ```bash
  # Debian/Ubuntu
  apt-get install p7zip-full genisoimage
  ```

> [!NOTE]
> The plugin has been tested with VMware Cloud Director 10.6.

## Usage

For examples on how to use this plugin with Packer refer to the [examples](examples/) directory of
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

For ISO-based builds with preseed/kickstart, the VM needs network connectivity during OS installation.

### IP Allocation Modes

| Mode | Description | Use Case |
|------|-------------|----------|
| `POOL` | VCD assigns IP from pool (default) | Networks with IP pools |
| `MANUAL` | User specifies IP via `vm_ip` | When you need a specific IP |
| `DHCP` | OS gets IP from DHCP server | Networks with DHCP enabled |

For `POOL` and `MANUAL` modes, the IP is available as template variables in `boot_command` and `cd_content`:
- `{{ .VMIP }}` - The VM's IP address
- `{{ .VMGateway }}` - Network gateway
- `{{ .VMNetmask }}` - Network mask
- `{{ .VMDNS }}` - DNS server

### POOL Mode (Recommended)

Let VCD assign an IP from the network pool. The plugin queries the assigned IP and makes it
available for templates:

```hcl
network            = "my-network"
ip_allocation_mode = "POOL"  # This is the default

# Debian preseed example
boot_command = [
  "<esc>auto ",
  "netcfg/disable_autoconfig=true ",
  "netcfg/get_ipaddress={{ .VMIP }} ",
  "netcfg/get_netmask={{ .VMNetmask }} ",
  "netcfg/get_gateway={{ .VMGateway }} ",
  "netcfg/get_nameservers={{ .VMDNS }} ",
  "preseed/url=http://{{ .HTTPIP }}:{{ .HTTPPort }}/preseed.cfg<enter>"
]
```

### MANUAL Mode

Specify the IP address explicitly:

```hcl
network            = "my-network"
ip_allocation_mode = "MANUAL"
vm_ip              = "10.0.0.100"
vm_gateway         = "10.0.0.1"
vm_dns             = "8.8.8.8"

boot_command = [
  "<esc>auto ",
  "netcfg/disable_autoconfig=true ",
  "netcfg/get_ipaddress={{ .VMIP }} ",
  "netcfg/get_netmask={{ .VMNetmask }} ",
  "netcfg/get_gateway={{ .VMGateway }} ",
  "netcfg/get_nameservers={{ .VMDNS }} ",
  "preseed/url=http://{{ .HTTPIP }}:{{ .HTTPPort }}/preseed.cfg<enter>"
]
```

### DHCP Mode

If your network has DHCP, the OS installer can get an IP automatically:

```hcl
network            = "my-network"
ip_allocation_mode = "DHCP"

boot_command = [
  "auto url=http://{{ .HTTPIP }}:{{ .HTTPPort }}/preseed.cfg<enter>"
]
```

> **Note:** The boot command examples use Debian preseed. Other distributions use different
> methods: kickstart for RHEL/CentOS/Fedora, autoinstall for Ubuntu 20.04+.

## Contributing

If you discover a bug or would like to suggest a feature or an enhancement, please use the GitHub
[issues][issues].

## GenAI Disclaimer

I have used Claude Code for this project. I have been working with VMware Cloud Director and govcd ([docker-machine-driver-vcd][docker-machine-driver-vcd], [fleeting-plugin-vcd][fleeting-plugin-vcd]) for years now, but this project was way bigger than and it had a major showstopper: VCD does not have an API call to "press keys" and send them to the VM, so in order to type the boot command it was necessary to reverse engineering the WebSockets Web Console and "type" them in the console. Claude helped A LOT.

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
