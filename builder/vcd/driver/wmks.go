package driver

import (
	"crypto/tls"
	"fmt"
	"time"

	"github.com/gorilla/websocket"
)

// WMKS message types (from wmks.min.js analysis)
const (
	// Message type marker for binary frames
	msgTypeBinary = 127

	// Subtypes for key events
	msgVMWKeyEvent  = 0 // 8-byte key event
	msgVMWKeyEvent2 = 6 // 10-byte key event with extended flags
)

// VScanCodes are PS/2 scan codes used by WMKS
// These are NOT USB HID codes - they're the legacy PC keyboard scan codes
var VScanCodes = map[string]int{
	"ESCAPE":    1,
	"1":         2,
	"2":         3,
	"3":         4,
	"4":         5,
	"5":         6,
	"6":         7,
	"7":         8,
	"8":         9,
	"9":         10,
	"0":         11,
	"MINUS":     12,
	"EQUALS":    13,
	"BACKSPACE": 14,
	"TAB":       15,
	"Q":         16,
	"W":         17,
	"E":         18,
	"R":         19,
	"T":         20,
	"Y":         21,
	"U":         22,
	"I":         23,
	"O":         24,
	"P":         25,
	"LBRACKET":  26,
	"RBRACKET":  27,
	"ENTER":     28,
	"LCTRL":     29,
	"A":         30,
	"S":         31,
	"D":         32,
	"F":         33,
	"G":         34,
	"H":         35,
	"J":         36,
	"K":         37,
	"L":         38,
	"SEMICOLON": 39,
	"QUOTE":     40,
	"BACKTICK":  41,
	"LSHIFT":    42,
	"BACKSLASH": 43,
	"Z":         44,
	"X":         45,
	"C":         46,
	"V":         47,
	"B":         48,
	"N":         49,
	"M":         50,
	"COMMA":     51,
	"PERIOD":    52,
	"SLASH":     53,
	"RSHIFT":    54,
	"MULTIPLY":  55, // Keypad *
	"LALT":      56,
	"SPACE":     57,
	"CAPSLOCK":  58,
	"F1":        59,
	"F2":        60,
	"F3":        61,
	"F4":        62,
	"F5":        63,
	"F6":        64,
	"F7":        65,
	"F8":        66,
	"F9":        67,
	"F10":       68,
	"NUMLOCK":   69,
	"SCROLLOCK": 70,
	"KP7":       71, // Keypad 7/Home
	"KP8":       72, // Keypad 8/Up
	"KP9":       73, // Keypad 9/PgUp
	"KPMINUS":   74, // Keypad -
	"KP4":       75, // Keypad 4/Left
	"KP5":       76, // Keypad 5
	"KP6":       77, // Keypad 6/Right
	"KPPLUS":    78, // Keypad +
	"KP1":       79, // Keypad 1/End
	"KP2":       80, // Keypad 2/Down
	"KP3":       81, // Keypad 3/PgDn
	"KP0":       82, // Keypad 0/Ins
	"KPDOT":     83, // Keypad ./Del
	"F11":       87,
	"F12":       88,
	// Extended keys (prefixed with 0xE0 in raw scan codes)
	"KPENTER":   0x11C, // Keypad Enter
	"RCTRL":     0x11D,
	"KPSLASH":   0x135, // Keypad /
	"RALT":      0x138,
	"HOME":      0x147,
	"UP":        0x148,
	"PAGEUP":    0x149,
	"LEFT":      0x14B,
	"RIGHT":     0x14D,
	"END":       0x14F,
	"DOWN":      0x150,
	"PAGEDOWN":  0x151,
	"INSERT":    0x152,
	"DELETE":    0x153,
}

// WMKSClient provides console access to a VM via WebMKS protocol
type WMKSClient struct {
	conn         *websocket.Conn
	ticket       *MksTicket
	insecure     bool
	connected    bool
	keyDelay     time.Duration
	groupDelay   time.Duration
	specialDelay time.Duration
	authToken    string // VCD authorization token
	authHeader   string // VCD auth header name (x-vcloud-authorization or Authorization)
}

// WMKSOption configures the WMKS client
type WMKSOption func(*WMKSClient)

// WithInsecure allows connecting to servers with invalid TLS certificates
func WithInsecure(insecure bool) WMKSOption {
	return func(c *WMKSClient) {
		c.insecure = insecure
	}
}

// WithKeyDelay sets the delay between individual key presses
func WithKeyDelay(d time.Duration) WMKSOption {
	return func(c *WMKSClient) {
		c.keyDelay = d
	}
}

// WithGroupDelay sets the delay after special key sequences
func WithGroupDelay(d time.Duration) WMKSOption {
	return func(c *WMKSClient) {
		c.groupDelay = d
	}
}

// WithAuth sets the VCD authentication token and header
func WithAuth(token, header string) WMKSOption {
	return func(c *WMKSClient) {
		c.authToken = token
		c.authHeader = header
	}
}

// NewWMKSClient creates a new WMKS client for the given ticket
func NewWMKSClient(ticket *MksTicket, opts ...WMKSOption) *WMKSClient {
	c := &WMKSClient{
		ticket:       ticket,
		keyDelay:     10 * time.Millisecond,
		groupDelay:   100 * time.Millisecond,
		specialDelay: 50 * time.Millisecond,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Connect establishes the WebSocket connection to the VM console
func (c *WMKSClient) Connect() error {
	wsURL := c.ticket.WebSocketURL()

	dialer := websocket.Dialer{
		Subprotocols: []string{"binary"}, // VCD console uses "binary" subprotocol
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: c.insecure,
		},
	}

	// Add required headers for WMKS handshake
	headers := make(map[string][]string)
	headers["Origin"] = []string{"https://" + c.ticket.Host}

	conn, resp, err := dialer.Dial(wsURL, headers)
	if err != nil {
		if resp != nil {
			return fmt.Errorf("failed to connect to WMKS at %s (status %d): %w", wsURL, resp.StatusCode, err)
		}
		return fmt.Errorf("failed to connect to WMKS at %s: %w", wsURL, err)
	}

	c.conn = conn

	// Complete RFB/VNC handshake - WMKS uses RFB protocol
	if err := c.rfbHandshake(); err != nil {
		conn.Close()
		return fmt.Errorf("RFB handshake failed: %w", err)
	}

	c.connected = true
	return nil
}

// rfbHandshake performs the RFB protocol negotiation
func (c *WMKSClient) rfbHandshake() error {
	// Step 1: Read server version (e.g., "RFB 003.008\n")
	_, versionMsg, err := c.conn.ReadMessage()
	if err != nil {
		return fmt.Errorf("failed to read server version: %w", err)
	}
	if len(versionMsg) < 12 || string(versionMsg[:4]) != "RFB " {
		return fmt.Errorf("invalid server version: %s", string(versionMsg))
	}

	// Step 2: Send client version (same as server)
	err = c.conn.WriteMessage(websocket.BinaryMessage, versionMsg)
	if err != nil {
		return fmt.Errorf("failed to send client version: %w", err)
	}

	// Step 3: Read security types
	_, secTypes, err := c.conn.ReadMessage()
	if err != nil {
		return fmt.Errorf("failed to read security types: %w", err)
	}
	if len(secTypes) < 1 {
		return fmt.Errorf("no security types received")
	}

	// First byte is count, rest are security type values
	// Type 1 = None (no authentication)
	// We'll select the first one
	numTypes := int(secTypes[0])
	if numTypes == 0 {
		// RFB 3.8 error - read reason
		return fmt.Errorf("server refused connection")
	}

	// Find security type 1 (None) or use first available
	selectedType := secTypes[1]
	for i := 1; i <= numTypes && i < len(secTypes); i++ {
		if secTypes[i] == 1 { // None
			selectedType = 1
			break
		}
	}

	// Step 4: Send selected security type
	err = c.conn.WriteMessage(websocket.BinaryMessage, []byte{selectedType})
	if err != nil {
		return fmt.Errorf("failed to send security type: %w", err)
	}

	// Step 5: If security type != 1 (None), handle authentication
	// For VCD console with ticket auth, it usually accepts None
	if selectedType != 1 {
		// Read security result
		_, secResult, err := c.conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("failed to read security result: %w", err)
		}
		if len(secResult) >= 4 && (secResult[0] != 0 || secResult[1] != 0 || secResult[2] != 0 || secResult[3] != 0) {
			return fmt.Errorf("security authentication failed")
		}
	}

	// For RFB 3.8 with security type 1, there's no security result

	// Step 6: Send ClientInit (1 = shared session)
	err = c.conn.WriteMessage(websocket.BinaryMessage, []byte{1})
	if err != nil {
		return fmt.Errorf("failed to send ClientInit: %w", err)
	}

	// Step 7: Read ServerInit (we don't need the details, just consume it)
	_, _, err = c.conn.ReadMessage()
	if err != nil {
		return fmt.Errorf("failed to read ServerInit: %w", err)
	}

	return nil
}

// Close closes the WebSocket connection
func (c *WMKSClient) Close() error {
	if c.conn != nil {
		c.connected = false
		return c.conn.Close()
	}
	return nil
}

// ReadMessage reads a single message from the WebSocket (for debugging)
func (c *WMKSClient) ReadMessage() ([]byte, error) {
	if !c.connected {
		return nil, fmt.Errorf("not connected")
	}
	c.conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	_, data, err := c.conn.ReadMessage()
	return data, err
}

// DrainMessages reads and discards any pending messages (non-blocking)
func (c *WMKSClient) DrainMessages() {
	for {
		c.conn.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

// SendKeyEvent sends a single key event (press or release)
func (c *WMKSClient) SendKeyEvent(scanCode int, down bool) error {
	if !c.connected {
		return fmt.Errorf("not connected")
	}

	// Build the key event message using msgVMWKeyEvent (8 bytes total)
	// Format from wmks.min.js:
	//   push8(127)      - msgVMWClientMessage
	//   push8(0)        - msgVMWKeyEvent
	//   push16(8)       - total message length
	//   push16(scancode)- VScanCode
	//   push8(down)     - 1=down, 0=up
	//   push8(0)        - flags
	msg := make([]byte, 8)
	msg[0] = msgTypeBinary  // 127
	msg[1] = msgVMWKeyEvent // 0
	msg[2] = 0              // length high byte
	msg[3] = 8              // length low byte (total message size = 8)
	msg[4] = byte((scanCode >> 8) & 0xFF)
	msg[5] = byte(scanCode & 0xFF)
	if down {
		msg[6] = 1
	} else {
		msg[6] = 0
	}
	msg[7] = 0 // flags

	return c.conn.WriteMessage(websocket.BinaryMessage, msg)
}

// SendKey sends a key press followed by release
func (c *WMKSClient) SendKey(scanCode int) error {
	if err := c.SendKeyEvent(scanCode, true); err != nil {
		return err
	}
	time.Sleep(c.keyDelay)
	return c.SendKeyEvent(scanCode, false)
}

// SendString types a string character by character
func (c *WMKSClient) SendString(s string) error {
	for _, char := range s {
		scanCode, shift := charToScanCode(char)
		if scanCode == 0 {
			continue // Skip unknown characters
		}

		if shift {
			if err := c.SendKeyEvent(VScanCodes["LSHIFT"], true); err != nil {
				return err
			}
			time.Sleep(c.keyDelay)
		}

		if err := c.SendKey(scanCode); err != nil {
			return err
		}

		if shift {
			time.Sleep(c.keyDelay)
			if err := c.SendKeyEvent(VScanCodes["LSHIFT"], false); err != nil {
				return err
			}
		}

		time.Sleep(c.keyDelay)
	}
	return nil
}

// charToScanCode converts a character to its scan code and whether shift is needed
func charToScanCode(char rune) (scanCode int, shift bool) {
	switch {
	case char >= 'a' && char <= 'z':
		return VScanCodes[string(char-32)], false // Convert to uppercase for lookup
	case char >= 'A' && char <= 'Z':
		return VScanCodes[string(char)], true
	case char >= '0' && char <= '9':
		return VScanCodes[string(char)], false
	case char == ' ':
		return VScanCodes["SPACE"], false
	case char == '\n' || char == '\r':
		return VScanCodes["ENTER"], false
	case char == '\t':
		return VScanCodes["TAB"], false
	case char == '-':
		return VScanCodes["MINUS"], false
	case char == '=':
		return VScanCodes["EQUALS"], false
	case char == '[':
		return VScanCodes["LBRACKET"], false
	case char == ']':
		return VScanCodes["RBRACKET"], false
	case char == ';':
		return VScanCodes["SEMICOLON"], false
	case char == '\'':
		return VScanCodes["QUOTE"], false
	case char == '`':
		return VScanCodes["BACKTICK"], false
	case char == '\\':
		return VScanCodes["BACKSLASH"], false
	case char == ',':
		return VScanCodes["COMMA"], false
	case char == '.':
		return VScanCodes["PERIOD"], false
	case char == '/':
		return VScanCodes["SLASH"], false
	// Shifted characters
	case char == '!':
		return VScanCodes["1"], true
	case char == '@':
		return VScanCodes["2"], true
	case char == '#':
		return VScanCodes["3"], true
	case char == '$':
		return VScanCodes["4"], true
	case char == '%':
		return VScanCodes["5"], true
	case char == '^':
		return VScanCodes["6"], true
	case char == '&':
		return VScanCodes["7"], true
	case char == '*':
		return VScanCodes["8"], true
	case char == '(':
		return VScanCodes["9"], true
	case char == ')':
		return VScanCodes["0"], true
	case char == '_':
		return VScanCodes["MINUS"], true
	case char == '+':
		return VScanCodes["EQUALS"], true
	case char == '{':
		return VScanCodes["LBRACKET"], true
	case char == '}':
		return VScanCodes["RBRACKET"], true
	case char == ':':
		return VScanCodes["SEMICOLON"], true
	case char == '"':
		return VScanCodes["QUOTE"], true
	case char == '~':
		return VScanCodes["BACKTICK"], true
	case char == '|':
		return VScanCodes["BACKSLASH"], true
	case char == '<':
		return VScanCodes["COMMA"], true
	case char == '>':
		return VScanCodes["PERIOD"], true
	case char == '?':
		return VScanCodes["SLASH"], true
	default:
		return 0, false
	}
}

// SendSpecialKey sends a special key by name (e.g., "ENTER", "F1", "ESCAPE")
func (c *WMKSClient) SendSpecialKey(keyName string) error {
	scanCode, ok := VScanCodes[keyName]
	if !ok {
		return fmt.Errorf("unknown special key: %s", keyName)
	}
	return c.SendKey(scanCode)
}
