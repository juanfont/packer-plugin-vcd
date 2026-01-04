# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Packer multi-component plugin for VMware Cloud Director (VCD). It enables automated VM creation from ISO images in VCD environments. The plugin uses the `go-vcloud-director` library to interact with the VCD API (version 38.1).

## Build Commands

```bash
make build         # Standard Go build
make dev           # Build with dev version and install to Packer plugins
make test          # Run unit tests with race detection (3m timeout)
make testacc       # Run acceptance tests (requires PACKER_ACC=1, 120m timeout)
make generate      # Regenerate HCL2 specs and documentation
make plugin-check  # Validate plugin compatibility with Packer SDK
```

To run a single test:
```bash
go test -race -count 1 -v ./path/to/package -run TestName -timeout=3m
```

## Architecture

### Plugin Registration (main.go)
The plugin registers a single builder `iso` via the Packer plugin SDK. The builder is referenced as `vcd-iso` in Packer configurations.

### Builder Pipeline (builder/vcd/iso/)
- **config.go**: Composes configuration from multiple embedded structs (ConnectConfig, LocationConfig, HardwareConfig, etc.). HCL2 specs are auto-generated via `go:generate` directives.
- **builder.go**: Implements the multistep build pipeline using `packer-plugin-sdk/multistep`.

### VCD Driver (builder/vcd/driver/)
- **driver.go**: Wraps `govcd.VCDClient` for VCD API operations. Supports username/password and token-based authentication.
- **vm.go**: `VirtualMachineDriver` interface for VM lifecycle operations (power, status, media, hardware).
- **console.go**: MKS ticket acquisition for VM console access.
- **wmks.go**: WebMKS protocol implementation for keyboard input via RFB/VNC protocol.
- **wmks_driver.go**: Implements Packer's `bootcommand.BCDriver` interface.

### Common Components (builder/vcd/common/)
Contains shared configuration structs and build steps:
- `StepConnect`: Establishes VCD connection
- `StepCreateTempCatalog`: Creates temporary catalog for ISO storage
- `StepUploadISO`: Uploads ISO to catalog
- `StepResolveVApp`: Gets or creates vApp container
- `StepMountISO`: Mounts ISO to VM CD-ROM
- `StepRun`: Powers on VM
- `StepBootCommand`: Sends boot commands via WMKS console
- `StepShutdown`: Graceful VM shutdown
- `StepExportToCatalog`: Exports vApp as template
- Configuration structs: `ConnectConfig`, `LocationConfig`, `HardwareConfig`, `CatalogConfig`, `RunConfig`, `ShutdownConfig`, `ExportConfig`, `ExportToCatalogConfig`, `BootCommandConfig`

## Code Generation

Files ending in `.hcl2spec.go` are auto-generated. After modifying config structs with `mapstructure` tags, run:
```bash
make generate
```

This requires `packer-sdc` which is installed via `make install-packer-sdc`.

## Key Dependencies

- `github.com/hashicorp/packer-plugin-sdk` v0.6.4 - Packer plugin framework
- `github.com/vmware/go-vcloud-director/v3` v3.0.0 - VCD API client
- `github.com/gorilla/websocket` - WebSocket client for WMKS console

## Implementation Progress

### Completed (Phase 1 & 2)

1. **VCD Connection Layer**
   - Username/password authentication
   - API token authentication
   - Organization and VDC resolution

2. **ISO Management**
   - ISO download (via Packer SDK)
   - Catalog creation with VDC storage profile alignment
   - ISO upload with progress tracking
   - Optional ISO caching

3. **VM Creation Pipeline**
   - vApp creation/resolution
   - Empty VM creation with hardware specs
   - CPU, memory, disk configuration
   - Network attachment
   - Firmware selection (BIOS/EFI)
   - ISO mounting to CD-ROM

4. **WMKS Boot Command**
   - MKS ticket acquisition via VCD API
   - WebSocket connection to VCD console proxy (wss://host:port/port;ticket format)
   - RFB handshake (version negotiation, security, client/server init)
   - PS/2 scan code keyboard mapping
   - Full Packer boot_command syntax support (<enter>, <wait>, <f1>-<f12>, etc.)

5. **VM Lifecycle**
   - Power on/off
   - Graceful shutdown with timeout
   - vApp export to catalog

### Remaining Work (Phase 3)

1. **HTTP Server for Kickstart/Preseed**
   - Serve files during boot for automated installation
   - Template variable interpolation ({{ .HTTPIP }}, {{ .HTTPPort }})

2. **SSH/WinRM Communicator**
   - Wait for OS installation to complete
   - Connect via SSH or WinRM
   - Run provisioners (shell, ansible, etc.)

3. **Testing & Documentation**
   - Integration tests with real VCD
   - Complete documentation
   - Example templates for common OS installations

4. **Polish**
   - Better error messages
   - Progress reporting
   - Cleanup on failure

## Testing Tools

The `cmd/vcdtest` tool provides commands for testing VCD functionality:
```bash
go run ./cmd/vcdtest                    # Basic connectivity test
go run ./cmd/vcdtest upload-iso <path>  # Test ISO upload
go run ./cmd/vcdtest list-networks      # List available networks
go run ./cmd/vcdtest create-vm          # Test VM creation
go run ./cmd/vcdtest full-test <iso>    # Full pipeline test
go run ./cmd/vcdtest console-test <vm>  # Test WMKS console
```

Requires environment variables: `VCD_HOST`, `VCD_USERNAME`, `VCD_PASSWORD`, `VCD_ORG`, `VCD_VDC`

## Key Technical Details

### Catalog Storage Profile Issue
When creating catalogs for ISO upload, the catalog must use a storage profile from the same VDC where VMs will be deployed. Otherwise, the media won't be accessible from ESXi hosts. Solution: `StepCreateTempCatalog` uses `AdminOrg.CreateCatalogWithStorageProfile()`.

### WMKS Protocol
VCD console uses RFB (VNC) protocol over WebSocket:
1. Server sends version: `RFB 003.008\n`
2. Client responds with same version
3. Security type negotiation (type 1 = None with ticket auth)
4. ClientInit/ServerInit exchange
5. Then WMKS key events can be sent (message type 127)

Key event format (8 bytes): `[127, 0, 0, 8, scancode_hi, scancode_lo, down, flags]`
