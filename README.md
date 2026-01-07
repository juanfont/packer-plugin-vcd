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

- **DHCP networks**: The installer will automatically obtain an IP address.
- **Static IP networks**: If your VCD network uses IP pools without DHCP, you must provide static
  network configuration via boot command parameters using the `vm_ip` configuration option.

## Contributing

If you discover a bug or would like to suggest a feature or an enhancement, please use the GitHub
[issues][issues].

## Related Projects

- [docker-machine-driver-vcd][docker-machine-driver-vcd] - Docker Machine driver for VCD
- [fleeting-plugin-vcd][fleeting-plugin-vcd] - GitLab fleeting plugin for VCD

## License

BSD-3-Clause

[vmware-vcd]: https://www.vmware.com/products/cloud-director.html
[packer-install]: https://developer.hashicorp.com/packer/install
[docs-packer-plugin-install]: https://developer.hashicorp.com/packer/docs/plugins/install-plugins
[docs-vcd-iso]: https://github.com/juanfont/packer-plugin-vcd/blob/main/docs/builders/vcd-iso.mdx
[issues]: https://github.com/juanfont/packer-plugin-vcd/issues
[docker-machine-driver-vcd]: https://github.com/juanfont/docker-machine-driver-vcd
[fleeting-plugin-vcd]: https://github.com/juanfont/fleeting-plugin-vcd
