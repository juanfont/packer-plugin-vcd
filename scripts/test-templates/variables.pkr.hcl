# Variables for VCD Packer tests
# Set these via environment variables (VCD_HOST, VCD_USERNAME, etc.) or .env file

packer {
  required_plugins {
    vcd = {
      version = ">= 0.0.3"
      source  = "github.com/juanfont/vcd"
    }
  }
}

variable "vcd_host" {
  type    = string
  default = env("VCD_HOST")
}

variable "vcd_username" {
  type    = string
  default = env("VCD_USERNAME")
}

variable "vcd_password" {
  type      = string
  sensitive = true
  default   = env("VCD_PASSWORD")
}

variable "vcd_org" {
  type    = string
  default = env("VCD_ORG")
}

variable "vcd_vdc" {
  type    = string
  default = env("VCD_VDC")
}

variable "vcd_network" {
  type    = string
  default = env("VCD_NETWORK")
}

variable "vcd_insecure" {
  type    = bool
  default = true
}

variable "vm_name" {
  type    = string
  default = "packer-test"
}

variable "ssh_username" {
  type    = string
  default = "packer"
}

variable "ssh_password" {
  type      = string
  sensitive = true
  default   = "packer"
}
