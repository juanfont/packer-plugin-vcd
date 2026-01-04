package common

import (
	"context"
	"fmt"
	"net"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
)

// StepHTTPIPDiscover discovers the host IP address to use for the HTTP server.
// This IP needs to be reachable from the VM for serving kickstart/preseed files.
type StepHTTPIPDiscover struct {
	// HTTPIP is a manually specified IP address to use
	HTTPIP string
	// HTTPInterface is a specific network interface to use
	HTTPInterface string
}

func (s *StepHTTPIPDiscover) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	ui := state.Get("ui").(packersdk.Ui)

	// If HTTP IP is manually specified, use it
	if s.HTTPIP != "" {
		ip := net.ParseIP(s.HTTPIP)
		if ip == nil {
			err := fmt.Errorf("invalid HTTP IP address: %s", s.HTTPIP)
			state.Put("error", err)
			ui.Error(err.Error())
			return multistep.ActionHalt
		}
		state.Put("http_ip", s.HTTPIP)
		ui.Sayf("Using configured HTTP IP: %s", s.HTTPIP)
		return multistep.ActionContinue
	}

	// If interface is specified, get IP from that interface
	if s.HTTPInterface != "" {
		ip, err := getIPFromInterface(s.HTTPInterface)
		if err != nil {
			state.Put("error", err)
			ui.Error(err.Error())
			return multistep.ActionHalt
		}
		state.Put("http_ip", ip)
		ui.Sayf("Using HTTP IP from interface %s: %s", s.HTTPInterface, ip)
		return multistep.ActionContinue
	}

	// Auto-discover: find first non-loopback IPv4 address
	ip, err := discoverHTTPIP()
	if err != nil {
		state.Put("error", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	state.Put("http_ip", ip)
	ui.Sayf("Discovered HTTP IP: %s", ip)
	return multistep.ActionContinue
}

func (s *StepHTTPIPDiscover) Cleanup(state multistep.StateBag) {
	// Nothing to clean up
}

// getIPFromInterface returns the first IPv4 address from the specified interface
func getIPFromInterface(ifaceName string) (string, error) {
	iface, err := net.InterfaceByName(ifaceName)
	if err != nil {
		return "", fmt.Errorf("failed to find interface %s: %w", ifaceName, err)
	}

	addrs, err := iface.Addrs()
	if err != nil {
		return "", fmt.Errorf("failed to get addresses for interface %s: %w", ifaceName, err)
	}

	for _, addr := range addrs {
		var ip net.IP
		switch v := addr.(type) {
		case *net.IPNet:
			ip = v.IP
		case *net.IPAddr:
			ip = v.IP
		}

		// Skip non-IPv4
		if ip == nil || ip.To4() == nil {
			continue
		}

		return ip.String(), nil
	}

	return "", fmt.Errorf("no IPv4 address found on interface %s", ifaceName)
}

// discoverHTTPIP finds the first non-loopback IPv4 address on the system
func discoverHTTPIP() (string, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return "", fmt.Errorf("failed to list network interfaces: %w", err)
	}

	for _, iface := range interfaces {
		// Skip loopback and down interfaces
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		if iface.Flags&net.FlagUp == 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}

			// Skip non-IPv4 and loopback
			if ip == nil || ip.To4() == nil || ip.IsLoopback() {
				continue
			}

			return ip.String(), nil
		}
	}

	return "", fmt.Errorf("no suitable IP address found for HTTP server")
}
