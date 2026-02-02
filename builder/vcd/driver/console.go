package driver

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"strings"

	"github.com/vmware/go-vcloud-director/v3/govcd"
	"github.com/vmware/go-vcloud-director/v3/types/v56"
)

// MksTicket represents the response from acquireMksTicket API
// This is used to establish a WebMKS console connection to a VM
type MksTicket struct {
	XMLName xml.Name `xml:"MksTicket"`
	Xmlns   string   `xml:"xmlns,attr,omitempty"`
	Host    string   `xml:"Host"`
	Port    int      `xml:"Port"`
	Ticket  string   `xml:"Ticket"`
	Vmx     string   `xml:"Vmx,omitempty"`
}

// AcquireMksTicket gets a WebMKS ticket for console access to a VM
// The ticket is valid for 30 seconds
func AcquireMksTicket(client *govcd.VCDClient, vm *govcd.VM) (*MksTicket, error) {
	// Find the acquireMksTicket link
	var ticketLink *types.Link
	for _, link := range vm.VM.Link {
		if link.Rel == types.RelScreenAcquireMksTicket {
			ticketLink = link
			break
		}
	}

	if ticketLink == nil {
		return nil, fmt.Errorf("VM does not have acquireMksTicket link - is it powered on?")
	}

	return acquireMksTicketFromURL(&client.Client, ticketLink.HREF)
}

// acquireMksTicketFromURL makes the actual API call to acquire an MKS ticket
// Uses govcd's ExecuteRequest for proper request/response handling
func acquireMksTicketFromURL(client *govcd.Client, ticketURL string) (*MksTicket, error) {
	ticket := &MksTicket{
		Xmlns: types.XMLNamespaceVCloud,
	}

	_, err := client.ExecuteRequest(
		ticketURL,
		http.MethodPost,
		"application/vnd.vmware.vcloud.mksticket+xml",
		"error acquiring MKS ticket: %s",
		nil,
		ticket,
	)
	if err != nil {
		return nil, err
	}

	return ticket, nil
}

// AcquireMksTicketDirect gets an MKS ticket using the VM's HREF directly
// This is an alternative method when the link traversal doesn't work
func AcquireMksTicketDirect(client *govcd.VCDClient, vmHref string) (*MksTicket, error) {
	ticketURL := strings.TrimSuffix(vmHref, "/") + "/screen/action/acquireMksTicket"
	return acquireMksTicketFromURL(&client.Client, ticketURL)
}

// WebSocketURL constructs the WebSocket URL for connecting to the VM console
func (t *MksTicket) WebSocketURL() string {
	// VCD 10.4+ console proxy URL format: wss://{host}:{port}/{port};{ticket}
	// The port appears twice: once in the host:port and once in the path with semicolon
	host := t.Host
	port := t.Port

	// Extract port from host if it includes one
	if strings.Contains(host, ":") {
		parts := strings.SplitN(host, ":", 2)
		host = parts[0]
		if port == 0 {
			fmt.Sscanf(parts[1], "%d", &port)
		}
	}

	// Default to 443 if no port specified
	if port == 0 {
		port = 443
	}

	// Ticket should start with / (e.g., /cst-xxx--tp-xxx--)
	ticket := t.Ticket
	if !strings.HasPrefix(ticket, "/") {
		ticket = "/" + ticket
	}

	// Format: wss://host:port/port;ticket
	return fmt.Sprintf("wss://%s:%d/%d;%s", host, port, port, ticket)
}
