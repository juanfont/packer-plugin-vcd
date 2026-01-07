packer {
  required_plugins {
    vcd = {
      version = ">= 0.0.1"
      source  = "github.com/juanfont/vcd"
    }
  }
}

source "vcd-iso" "debian-12" {
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
  vm_name       = var.vm_name
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

  # Export to catalog
  export_to_catalog {
    catalog        = var.export_catalog
    template_name  = var.template_name
    description    = var.template_description
    create_catalog = true
  }
}

build {
  sources = ["source.vcd-iso.debian-12"]

  provisioner "shell" {
    inline = [
      "echo 'Debian 12 VM provisioned successfully!'",
      "uname -a",
      "cat /etc/os-release"
    ]
  }
}
