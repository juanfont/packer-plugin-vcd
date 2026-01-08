packer {
  required_plugins {
    vcd = {
      version = ">= 0.0.1"
      source  = "github.com/juanfont/vcd"
    }
  }
}

source "vcd-iso" "debian-12-floppy-test" {
  # VCD Connection
  host                = var.vcd_host
  username            = var.vcd_username
  password            = var.vcd_password
  org                 = var.vcd_org
  vdc                 = var.vcd_vdc
  insecure_connection = var.vcd_insecure

  # ISO Configuration
  iso_url      = "https://cdimage.debian.org/cdimage/archive/12.9.0/amd64/iso-cd/debian-12.9.0-amd64-netinst.iso"
  iso_checksum = "sha256:1257373c706d8c07e6917942736a865dfff557d21d76ea3040bb1039eb72a054"

  # VM Configuration
  vm_name       = "packer-floppy-test"
  guest_os_type = "debian12_64Guest"
  CPUs          = 2
  memory        = 2048
  disk_size_mb  = 20480

  # Network - using auto_discover_ip for static IP networks
  network            = var.vcd_network
  ip_allocation_mode = "MANUAL"
  auto_discover_ip   = true

  # HTTP server for preseed
  http_directory = "http"

  # Floppy configuration - test file
  floppy_content = {
    "test.txt"       = "Hello from floppy disk! This is a test file created by packer-plugin-vcd."
    "config/app.cfg" = "[settings]\nkey=value\ntest=true"
  }
  floppy_label = "TESTFLP"

  # Boot command for Debian installer with auto-discovered network info
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

  # SSH Configuration
  ssh_username = var.ssh_username
  ssh_password = var.ssh_password
  ssh_timeout  = "30m"

  # Shutdown
  shutdown_command = "echo '${var.ssh_password}' | sudo -S shutdown -P now"
}

build {
  sources = ["source.vcd-iso.debian-12-floppy-test"]

  # Test that floppy content is accessible
  provisioner "shell" {
    inline = [
      "echo '=== Checking for floppy device ==='",
      "lsblk -a || true",
      "echo ''",
      "echo '=== Checking /dev/fd* devices ==='",
      "ls -la /dev/fd* 2>/dev/null || echo 'No /dev/fd* devices found'",
      "echo ''",
      "echo '=== Checking mounted media ==='",
      "mount | grep -E 'cd|dvd|sr|fd|floppy' || echo 'No CD/floppy mounts found'",
      "echo ''",
      "echo '=== Attempting to mount floppy/media ==='",
      "sudo mkdir -p /mnt/floppy",
      "sudo mount /dev/fd0 /mnt/floppy 2>/dev/null && echo 'Mounted /dev/fd0' || echo 'Could not mount /dev/fd0'",
      "echo ''",
      "echo '=== Checking CD-ROM for floppy content ==='",
      "sudo mkdir -p /mnt/cdrom",
      "sudo mount /dev/sr1 /mnt/cdrom 2>/dev/null && echo 'Mounted /dev/sr1' || echo 'Could not mount /dev/sr1'",
      "sudo mount /dev/sr0 /mnt/cdrom 2>/dev/null && echo 'Mounted /dev/sr0' || echo 'sr0 already mounted or unavailable'",
      "echo ''",
      "echo '=== Listing mounted content ==='",
      "ls -la /mnt/floppy/ 2>/dev/null || echo '/mnt/floppy empty or not mounted'",
      "ls -la /mnt/cdrom/ 2>/dev/null || echo '/mnt/cdrom empty or not mounted'",
      "echo ''",
      "echo '=== Reading test.txt ==='",
      "cat /mnt/floppy/test.txt 2>/dev/null || cat /mnt/cdrom/test.txt 2>/dev/null || echo 'test.txt not found'",
      "echo ''",
      "echo '=== Reading config/app.cfg ==='",
      "cat /mnt/floppy/config/app.cfg 2>/dev/null || cat /mnt/cdrom/config/app.cfg 2>/dev/null || echo 'config/app.cfg not found'",
      "echo ''",
      "echo '=== Floppy test complete ==='"
    ]
  }
}
