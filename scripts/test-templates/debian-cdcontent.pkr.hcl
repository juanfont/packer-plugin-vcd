# Debian 12 with cd_content test - ISO modification
# Tests: ISO modification, boot preservation, VM creation, preseed, SSH

source "vcd-iso" "debian-cdcontent" {
  # VCD Connection
  host                = var.vcd_host
  username            = var.vcd_username
  password            = var.vcd_password
  org                 = var.vcd_org
  vdc                 = var.vcd_vdc
  insecure_connection = var.vcd_insecure

  # ISO - will be modified with cd_content
  iso_url      = "https://cdimage.debian.org/cdimage/archive/12.9.0/amd64/iso-cd/debian-12.9.0-amd64-netinst.iso"
  iso_checksum = "sha256:1257373c706d8c07e6917942736a865dfff557d21d76ea3040bb1039eb72a054"

  # CD Content - files to add to the ISO
  cd_content = {
    "packer-test.txt"      = "This file was added by packer cd_content"
    "config/settings.conf" = "[packer]\ntest=true\nversion=0.0.4"
  }

  # VM Configuration
  vm_name       = "${var.vm_name}-debian-cdcontent"
  guest_os_type = "debian12_64Guest"
  CPUs          = 2
  memory        = 2048
  disk_size_mb  = 20480

  # Network
  network            = var.vcd_network
  ip_allocation_mode = "MANUAL"
  auto_discover_ip   = true

  # HTTP server for preseed
  http_directory = "http"

  # Boot command
  boot_wait = "5s"
  boot_command = [
    "<esc><wait>",
    "auto ",
    "netcfg/disable_autoconfig=true ",
    "netcfg/get_ipaddress={{ .VMIP }} ",
    "netcfg/get_netmask={{ .Netmask }} ",
    "netcfg/get_gateway={{ .Gateway }} ",
    "netcfg/get_nameservers={{ .DNS }} ",
    "preseed/url=http://{{ .HTTPIP }}:{{ .HTTPPort }}/preseed.cfg ",
    "<enter>"
  ]

  # SSH
  ssh_username = var.ssh_username
  ssh_password = var.ssh_password
  ssh_timeout  = "30m"

  # Shutdown
  shutdown_command = "echo '${var.ssh_password}' | sudo -S shutdown -P now"
}

build {
  sources = ["source.vcd-iso.debian-cdcontent"]

  provisioner "shell" {
    inline = [
      "echo '=== Debian cd_content Test ==='",
      "echo 'Hostname:' $(hostname)",
      "echo 'Kernel:' $(uname -r)",
      "echo ''",
      "echo '=== Mounting CD-ROM to verify cd_content files ==='",
      "sudo mkdir -p /mnt/cdrom",
      "sudo mount /dev/sr0 /mnt/cdrom || true",
      "echo ''",
      "echo '=== Listing root of CD-ROM ==='",
      "ls -la /mnt/cdrom/ | head -20",
      "echo ''",
      "echo '=== Checking for packer-test.txt ==='",
      "ls -la /mnt/cdrom/packer-test.txt || echo 'File not found'",
      "sudo cat /mnt/cdrom/packer-test.txt || echo 'Cannot read file'",
      "echo ''",
      "echo '=== Checking for config/settings.conf ==='",
      "ls -la /mnt/cdrom/config/ || echo 'config dir not found'",
      "sudo cat /mnt/cdrom/config/settings.conf || echo 'Cannot read file'",
      "echo ''",
      "echo '=== cd_content verification complete ==='",
      "sudo umount /mnt/cdrom || true"
    ]
  }
}
