# Windows 11 test for VCD
# Tests: EFI firmware, TPM, cd_content with autounattend.xml, WinRM
#
# NOTE: Windows ISOs use UDF filesystem. Modifying them requires external tools:
#   apt-get install p7zip-full genisoimage

locals {
  # Autounattend template variables
  build_username       = "packer"
  build_password       = "Packer123!"
  vm_guest_os_language = "en-US"
  vm_computer_name     = "WIN11"
  vm_guest_os_timezone = "UTC"
}

source "vcd-iso" "windows11" {
  # VCD Connection
  host                = var.vcd_host
  username            = var.vcd_username
  password            = var.vcd_password
  org                 = var.vcd_org
  vdc                 = var.vcd_vdc
  insecure_connection = var.vcd_insecure

  # Windows 11 ISO
  iso_url      = "http://10.17.23.22/packer/SW_DVD9_Win_Pro_11_24H2.6_64BIT_English_Pro_Ent_EDU_N_MLF_X24-01686.ISO"
  iso_checksum = "sha256:1ebef64a1b263f5541c97ae0dc1723197310fe747a70aad39538d7d5f217f8da"

  # Inject Autounattend.xml and scripts into the ISO using cd_content
  # NOTE: Filename must be "Autounattend.xml" (capital A) for Windows auto-detection
  # NOTE: Network vars ({{ .VMIP }}, {{ .VMGateway }}, {{ .VMDNS }}) are substituted by
  #       the plugin at runtime after IP discovery. Other vars use Packer templatefile().
  cd_content = {
    "Autounattend.xml"      = templatefile("${path.root}/windows11/autounattend-winrm.xml", {
      build_username       = local.build_username
      build_password       = local.build_password
      vm_guest_os_language = local.vm_guest_os_language
      vm_computer_name     = local.vm_computer_name
      vm_guest_os_timezone = local.vm_guest_os_timezone
    })
    "vm-guest-tools.ps1"    = file("${path.root}/windows11/vm-guest-tools.ps1")
    "enable-winrm.ps1"      = file("${path.root}/windows11/enable-winrm.ps1")
  }


  # VM Configuration
  vm_name         = "${var.vm_name}-windows11"
  guest_os_type   = "windows11_64Guest"
  storage_profile = "ESR-Tier-3"
  CPUs            = 4
  memory          = 8192
  disk_size_mb    = 65536

  # EFI + TPM (required for Windows 11)
  # boot_delay=10s (VCD max) - times with Packer's 10s default wait
  firmware   = "efi-secure"
  vTPM       = true
  boot_delay = 10

  # Network - auto-discover IP
  network            = var.vcd_network
  ip_allocation_mode = "POOL"
  vm_gateway         = "10.17.23.1"
  vm_dns             = "8.8.8.8"

  # Boot command strategy:
  # 1. Wait for EFI boot menu to appear (after "Press any key" times out)
  # 2. Press Enter to select boot from CD
  # 3. Spam Enter to catch the new "Press any key to boot from CD" prompt
  boot_wait    = "40s"  # Wait longer for EFI menu to fully appear
  boot_command = [
    "<enter>",           # Select boot option in EFI menu
    "<wait2s><enter>",   # Catch "Press any key"
    "<wait1s><enter>",
    "<wait1s><enter>",
    "<wait1s><enter>",
    "<wait1s><enter>",
    "<wait1s><enter>",
    "<wait1s><enter>",
    "<wait1s><enter>",
    "<wait1s><enter>",
    "<wait1s><enter>"
  ]

  # WinRM communicator
  communicator   = "winrm"
  winrm_username = "packer"
  winrm_password = "Packer123!"
  winrm_timeout  = "2h"

  # Shutdown
  shutdown_command = "shutdown /s /t 10 /f /d p:4:1 /c \"Packer Shutdown\""
  shutdown_timeout = "30m"
}

build {
  sources = ["source.vcd-iso.windows11"]

  provisioner "powershell" {
    inline = [
      "Write-Host '=== Windows 11 VCD Test ==='",
      "Write-Host \"Hostname: $env:COMPUTERNAME\"",
      "Write-Host \"OS: $((Get-CimInstance Win32_OperatingSystem).Caption)\"",
      "Write-Host \"TPM: $((Get-Tpm).TpmPresent)\"",
      "Write-Host '=== Test PASSED ==='"
    ]
  }
}
