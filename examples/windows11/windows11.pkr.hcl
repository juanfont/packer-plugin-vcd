# Windows 11 Example for VCD
# Requires: EFI firmware, TPM, WinRM

locals {
  build_username = "packer"
  build_password = "Packer123!"
}

source "vcd-iso" "windows11" {
  # VCD Connection (use environment variables or var file)
  host                = var.vcd_host
  username            = var.vcd_username
  password            = var.vcd_password
  org                 = var.vcd_org
  vdc                 = var.vcd_vdc
  insecure_connection = var.vcd_insecure

  # ISO
  iso_url      = var.iso_url
  iso_checksum = var.iso_checksum

  # Inject Autounattend.xml into ISO (VCD only has one media slot)
  cd_content = {
    "Autounattend.xml" = templatefile("${path.root}/autounattend.xml", {
      build_username = local.build_username
      build_password = local.build_password
    })
    "enable-winrm.ps1" = file("${path.root}/enable-winrm.ps1")
  }

  # VM Configuration
  vm_name         = var.vm_name
  guest_os_type   = "windows11_64Guest"
  storage_profile = var.storage_profile
  CPUs            = 4
  memory          = 8192
  disk_size_mb    = 65536

  # EFI + TPM (required for Windows 11)
  firmware   = "efi-secure"
  vTPM       = true
  boot_delay = 10

  # Network
  network            = var.vcd_network
  ip_allocation_mode = "MANUAL"
  auto_discover_ip   = true
  vm_dns             = var.vm_dns

  # Boot command
  boot_wait    = "40s"
  boot_command = ["<enter><wait2s><enter><wait1s><enter><wait1s><enter>"]

  # WinRM
  communicator   = "winrm"
  winrm_username = local.build_username
  winrm_password = local.build_password
  winrm_timeout  = "2h"

  shutdown_command = "shutdown /s /t 10 /f /d p:4:1"
  shutdown_timeout = "30m"
}

build {
  sources = ["source.vcd-iso.windows11"]
}
