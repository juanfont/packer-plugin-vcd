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
The plugin registers a single builder `iso` via the Packer plugin SDK. The builder is referenced as `vcd.iso` in Packer configurations.

### Builder Pipeline (builder/vcd/iso/)
- **config.go**: Composes configuration from multiple embedded structs (ConnectConfig, LocationConfig, HardwareConfig, etc.). HCL2 specs are auto-generated via `go:generate` directives.
- **builder.go**: Implements the multistep build pipeline using `packer-plugin-sdk/multistep`.

### VCD Driver (builder/vcd/driver/)
- **driver.go**: Wraps `govcd.VCDClient` for VCD API operations. Supports username/password and token-based authentication.
- **vm.go**: `VirtualMachineDriver` interface for VM lifecycle operations.

### Common Components (builder/vcd/common/)
Contains shared configuration structs and build steps:
- `StepConnect`: Establishes VCD connection
- Configuration structs: `ConnectConfig`, `LocationConfig`, `HardwareConfig`, `RunConfig`, `ShutdownConfig`, `ExportConfig`, `ExportToCatalogConfig`

## Code Generation

Files ending in `.hcl2spec.go` are auto-generated. After modifying config structs with `mapstructure` tags, run:
```bash
make generate
```

This requires `packer-sdc` which is installed via `make install-packer-sdc`.

## Key Dependencies

- `github.com/hashicorp/packer-plugin-sdk` v0.6.4 - Packer plugin framework
- `github.com/vmware/go-vcloud-director/v3` v3.0.0 - VCD API client
