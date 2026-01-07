# Packer VCD Plugin Examples

This directory contains example templates for the Packer VCD plugin.

## Debian 12 Example

The `debian-12.pkr.hcl` template creates a Debian 12 VM from the official netinst ISO using:
- Automatic IP discovery from network pool (`auto_discover_ip = true`)
- Preseed-based unattended installation
- SSH provisioning

### Usage

1. Copy `.env.example` to `.env` and fill in your VCD credentials:

```bash
cp .env.example .env
# Edit .env with your values
```

2. Source the environment file:

```bash
source .env
```

3. Initialize and run Packer:

```bash
packer init debian-12.pkr.hcl
packer build debian-12.pkr.hcl
```

### Network Requirements

This example uses `auto_discover_ip = true` which requires:
- `ip_allocation_mode = "MANUAL"`
- A VCD network with a configured IP pool
- Network connectivity between Packer host and VCD network (for preseed HTTP server)

The boot command uses template variables to configure the VM's static IP:
- `{{ .VMIP }}` - Auto-discovered available IP from the pool
- `{{ .Gateway }}` - Network gateway
- `{{ .Netmask }}` - Network mask
- `{{ .DNS }}` - DNS server

### Files

- `debian-12.pkr.hcl` - Main Packer template
- `variables.pkr.hcl` - Variable definitions
- `http/preseed.cfg` - Debian preseed configuration
- `.env.example` - Example environment file
