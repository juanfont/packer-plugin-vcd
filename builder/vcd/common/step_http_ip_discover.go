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
	// HTTPIP is a manually specified IP address to use (overrides auto-discovery)
	HTTPIP string
	// HTTPInterface is a specific network interface to use
	HTTPInterface string
	// TargetHost is the host to use for route-based IP discovery (typically VCD host)
	TargetHost string
}

func (s *StepHTTPIPDiscover) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	ui := state.Get("ui").(packersdk.Ui)

	// If HTTP IP is manually specified and is a real routable IP, use it
	// Skip 0.0.0.0 as it's only valid for binding, not for VM connections
	if s.HTTPIP != "" && s.HTTPIP != "0.0.0.0" {
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

	// Auto-discover using route-based detection
	// This finds the local IP that would be used to reach the target host
	ip, err := discoverHTTPIP(s.TargetHost)
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

// discoverHTTPIP finds the local IP address that would be used to reach the target host.
// This uses the OS routing table by creating a UDP "connection" (no actual packets sent).
// This is the same approach used by packer-plugin-vsphere.
func discoverHTTPIP(targetHost string) (string, error) {
	// If no target host specified, use a well-known public IP
	// This will find the default route interface
	if targetHost == "" {
		targetHost = "8.8.8.8"
	}

	// Resolve hostname to IP if needed
	target := targetHost
	if net.ParseIP(targetHost) == nil {
		// It's a hostname, resolve it
		addrs, err := net.LookupHost(targetHost)
		if err != nil {
			return "", fmt.Errorf("failed to resolve target host %s: %w", targetHost, err)
		}
		if len(addrs) == 0 {
			return "", fmt.Errorf("no addresses found for target host %s", targetHost)
		}
		target = addrs[0]
	}

	// Create a UDP "connection" to the target
	// This doesn't actually send any packets, but uses the OS routing table
	// to determine which local interface would be used
	conn, err := net.Dial("udp", net.JoinHostPort(target, "80"))
	if err != nil {
		return "", fmt.Errorf("failed to determine route to %s: %w", targetHost, err)
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String(), nil
}
