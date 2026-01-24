# Packer VCD Plugin Examples

Example templates for the Packer VCD plugin.

## Quick Start

1. Copy `.env.example` to `.env` and fill in your VCD credentials
2. Source the environment: `source .env`
3. Run: `packer build -var-file=../variables.pkr.hcl <template>.pkr.hcl`

## Examples

- **debian/** - Debian 12 with preseed, SSH provisioning
- **windows11/** - Windows 11 with EFI + TPM, WinRM provisioning

## Template Variables for cd_content

When using `cd_content` with `templatefile()`, the plugin provides runtime variables for network configuration:

| Variable | Description |
|----------|-------------|
| `{{ .VMIP }}` | VM's assigned IP |
| `{{ .VMGateway }}` | Network gateway |
| `{{ .VMNetmask }}` | Network mask |
| `{{ .VMDNS }}` | DNS server |

Since `templatefile()` also uses `{{ }}` syntax, escape plugin variables:

```xml
<IpAddress>{{ "{{" }} .VMIP {{ "}}" }}/24</IpAddress>
```

Files loaded with `file()` don't need escaping.
