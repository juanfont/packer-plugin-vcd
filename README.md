# Packer Plugin for VMware Cloud Director (VCD)

A [Packer](https://www.packer.io/) plugin for building VM images on VMware Cloud Director (VCD). This plugin enables automated VM creation from ISO images with full boot command support for unattended installations.

## Features

- **ISO-based VM creation**: Upload ISOs to VCD catalogs and create VMs from scratch
- **Boot command support**: Send keystrokes to VM console via WebMKS protocol for automated OS installation
- **Catalog management**: Automatic temporary catalog creation with optional ISO caching
- **Export options**: Export finished VMs to OVF or VCD catalog templates

## Status

This plugin is under active development. Current implementation status:

| Feature | Status |
|---------|--------|
| VCD connection (username/password, token) | Done |
| ISO download and upload to catalog | Done |
| VM creation with hardware configuration | Done |
| Boot command via WMKS console | Done |
| HTTP server for kickstart/preseed files | Done |
| SSH/WinRM communicator for provisioning | Done |
| VM shutdown and cleanup | Done |
| Export to catalog | Done |

## Requirements

- [Packer](https://www.packer.io/downloads) >= 1.10.2
- [Go](https://golang.org/doc/install) >= 1.20 (for building from source)
- VMware Cloud Director 10.4+ (tested on 10.6)

### Network Requirements

For ISO-based builds with preseed/kickstart, the VM needs network connectivity to fetch the preseed file from the Packer HTTP server during OS installation.

**Option 1: DHCP Network (Recommended)**

Use a VCD network with DHCP enabled. The installer will automatically obtain an IP address and fetch the preseed file.

**Option 2: Static IP Configuration**

If your VCD network uses IP pools without DHCP, you must provide static network configuration via boot command parameters. The installer cannot auto-discover pool-assigned IPs.

Example boot command with static IP:
```hcl
boot_command = [
  "<esc>auto ",
  "netcfg/disable_autoconfig=true ",
  "netcfg/get_ipaddress=${var.vm_ip} ",
  "netcfg/get_netmask=${var.vm_netmask} ",
  "netcfg/get_gateway=${var.vm_gateway} ",
  "netcfg/get_nameservers=${var.vm_dns} ",
  "preseed/url=http://{{ .HTTPIP }}:{{ .HTTPPort }}/preseed.cfg<enter>"
]
```

**Note:** VCD "pool" networks allocate static IPs from a range but do not provide DHCP service. The VM will have an IP assigned in VCD's database, but the guest OS has no way to discover it during installation.

## Installation

### From Source

```bash
git clone https://github.com/juanfont/packer-plugin-vcd.git
cd packer-plugin-vcd
make dev
```

### Binary Release

Coming soon.

## Usage

### Basic Example

```hcl
packer {
  required_plugins {
    vcd = {
      source  = "github.com/juanfont/vcd"
      version = ">= 0.1.0"
    }
  }
}

source "vcd-iso" "debian" {
  # VCD Connection
  vcd_url      = "https://vcd.example.com/api"
  username     = "admin"
  password     = "secret"
  org          = "my-org"
  vdc          = "my-vdc"
  insecure     = true

  # ISO Configuration
  iso_url      = "https://cdimage.debian.org/debian-cd/current/amd64/iso-cd/debian-12.5.0-amd64-netinst.iso"
  iso_checksum = "sha256:..."

  # Catalog Configuration
  iso_catalog  = "my-iso-catalog"
  cache_iso    = true

  # VM Configuration
  vm_name         = "debian-template"
  guest_os_type   = "debian10_64Guest"
  cpus            = 2
  memory          = 2048
  disk_size_mb    = 20480
  network         = "my-network"

  # Boot Command
  boot_wait = "5s"
  boot_command = [
    "<esc><wait>",
    "auto url=http://{{ .HTTPIP }}:{{ .HTTPPort }}/preseed.cfg<enter>"
  ]

  # Export
  export_to_catalog {
    catalog     = "templates"
    name        = "debian-12-template"
    description = "Debian 12 base template"
  }
}

build {
  sources = ["source.vcd-iso.debian"]
}
```

## Configuration Reference

### VCD Connection

| Parameter | Required | Description |
|-----------|----------|-------------|
| `vcd_url` | Yes | VCD API URL |
| `username` | Yes* | VCD username |
| `password` | Yes* | VCD password |
| `token` | Yes* | VCD API token (alternative to username/password) |
| `org` | Yes | VCD organization |
| `vdc` | Yes | Virtual datacenter name |
| `insecure` | No | Skip TLS verification (default: false) |

*Either username/password or token is required.

### ISO Configuration

| Parameter | Required | Description |
|-----------|----------|-------------|
| `iso_url` | Yes | URL to download the ISO |
| `iso_checksum` | Yes | ISO checksum (e.g., `sha256:...`) |
| `iso_catalog` | No | Catalog for ISO storage (created if not exists) |
| `cache_iso` | No | Keep ISO in catalog after build (default: false) |

### VM Configuration

| Parameter | Required | Description |
|-----------|----------|-------------|
| `vm_name` | Yes | Name of the VM to create |
| `guest_os_type` | Yes | VCD guest OS type identifier |
| `cpus` | No | Number of CPUs (default: 1) |
| `memory` | No | Memory in MB (default: 1024) |
| `disk_size_mb` | No | Primary disk size in MB (default: 10240) |
| `network` | No | Network to attach |
| `storage_profile` | No | Storage profile name |
| `firmware` | No | Firmware type: `bios` or `efi` (default: bios) |

### Boot Command

| Parameter | Required | Description |
|-----------|----------|-------------|
| `boot_command` | No | List of boot commands to type |
| `boot_wait` | No | Time to wait before typing boot command |
| `boot_key_interval` | No | Time between keystrokes (default: 100ms) |

Boot command supports standard Packer syntax: `<enter>`, `<esc>`, `<tab>`, `<f1>`-`<f12>`, `<wait>`, `<waitXs>`, etc.

### Communicator (SSH/WinRM)

The plugin supports both SSH (Linux) and WinRM (Windows) communicators via the Packer SDK.

**SSH (default):**

| Parameter | Required | Description |
|-----------|----------|-------------|
| `ssh_username` | Yes | SSH username |
| `ssh_password` | No | SSH password |
| `ssh_private_key_file` | No | Path to SSH private key |
| `ssh_timeout` | No | SSH connection timeout (default: 5m) |

**WinRM (for Windows):**

| Parameter | Required | Description |
|-----------|----------|-------------|
| `communicator` | Yes | Set to `"winrm"` for Windows |
| `winrm_username` | Yes | WinRM username (usually "Administrator") |
| `winrm_password` | Yes | WinRM password |
| `winrm_timeout` | No | Connection timeout (default: 30m) |
| `winrm_port` | No | WinRM port (default: 5985, or 5986 for SSL) |
| `winrm_use_ssl` | No | Use HTTPS (default: false) |
| `winrm_insecure` | No | Skip certificate validation |
| `winrm_use_ntlm` | No | Use NTLM auth instead of basic |

**Windows Example:**

```hcl
source "vcd-iso" "windows" {
  # ... VCD and VM configuration ...

  communicator   = "winrm"
  winrm_username = "Administrator"
  winrm_password = "YourPassword"
  winrm_timeout  = "1h"
}
```

**Note:** For Windows, your `autounattend.xml` must enable WinRM:
```xml
<SynchronousCommand>
  <CommandLine>winrm quickconfig -q</CommandLine>
</SynchronousCommand>
<SynchronousCommand>
  <CommandLine>winrm set winrm/config/service @{AllowUnencrypted="true"}</CommandLine>
</SynchronousCommand>
<SynchronousCommand>
  <CommandLine>winrm set winrm/config/service/auth @{Basic="true"}</CommandLine>
</SynchronousCommand>
```

### Export to Catalog

Export the built VM as a vApp template to a VCD catalog.

| Parameter | Required | Description |
|-----------|----------|-------------|
| `catalog` | Yes | Target catalog name |
| `template_name` | No | Template name (defaults to VM name) |
| `description` | No | Template description |
| `overwrite` | No | Overwrite existing template (default: false) |
| `create_catalog` | No | Create catalog if it doesn't exist (default: false) |

**Example:**
```hcl
export_to_catalog {
  catalog        = "my-templates"
  template_name  = "debian-12-base"
  description    = "Debian 12 base template"
  create_catalog = true
  overwrite      = true
}
```

## Development

```bash
# Build
make build

# Run tests
make test

# Regenerate HCL2 specs after config changes
make generate

# Install for local testing
make dev
```

### Testing Console Connection

The `vcdtest` tool can be used to test WMKS console connectivity:

```bash
go run ./cmd/vcdtest console-test "https://vcd.example.com/api/vApp/vm-xxx" --text "hello" --enter
```

## Related Projects

- [docker-machine-driver-vcd](https://github.com/juanfont/docker-machine-driver-vcd) - Docker Machine driver for VCD
- [fleeting-plugin-vcd](https://github.com/juanfont/fleeting-plugin-vcd) - GitLab fleeting plugin for VCD

## Disclaimer

This project was developed using [Claude Code](https://claude.ai/code) (Anthropic's AI coding assistant). While the author has extensive experience with the VCD API from previous projects ([docker-machine-driver-vcd](https://github.com/juanfont/docker-machine-driver-vcd), [fleeting-plugin-vcd](https://github.com/juanfont/fleeting-plugin-vcd), and internal projects), this is the first project built primarily with Claude Code assistance. The initial scaffolding was generated from HashiCorp's packer-plugin-scaffolding template.

## License

BSD-3-Clause
