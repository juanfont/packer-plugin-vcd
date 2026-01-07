# VCD Connection
variable "vcd_host" {
  type        = string
  description = "VCD API endpoint hostname (without https://)"
}

variable "vcd_username" {
  type        = string
  description = "VCD username"
}

variable "vcd_password" {
  type        = string
  sensitive   = true
  description = "VCD password"
}

variable "vcd_org" {
  type        = string
  description = "VCD organization"
}

variable "vcd_vdc" {
  type        = string
  description = "VCD virtual datacenter"
}

variable "vcd_network" {
  type        = string
  description = "VCD network name"
}

variable "vcd_insecure" {
  type        = bool
  default     = true
  description = "Allow insecure TLS connections"
}

# VM Configuration
variable "vm_name" {
  type        = string
  default     = "debian-12-template"
  description = "Name of the VM to create"
}

variable "ssh_username" {
  type        = string
  default     = "packer"
  description = "SSH username for provisioning"
}

variable "ssh_password" {
  type        = string
  default     = "packer"
  sensitive   = true
  description = "SSH password for provisioning"
}
