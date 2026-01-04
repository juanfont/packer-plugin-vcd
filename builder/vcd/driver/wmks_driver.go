package driver

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"
	"unicode"

	"github.com/hashicorp/packer-plugin-sdk/bootcommand"
)

const shiftedChars = "~!@#$%^&*()_+{}|:\"<>?"

// WMKSBootDriver implements bootcommand.BCDriver for WMKS console
type WMKSBootDriver struct {
	client      *WMKSClient
	interval    time.Duration
	specialMap  map[string]int // maps special key names to scan codes
	scancodeMap map[rune]int   // maps characters to scan codes
}

// NewWMKSBootDriver creates a boot command driver that sends keystrokes via WMKS
func NewWMKSBootDriver(client *WMKSClient, interval time.Duration) *WMKSBootDriver {
	// Default key interval from environment or use default
	keyInterval := 100 * time.Millisecond
	if delay, err := time.ParseDuration(os.Getenv(bootcommand.PackerKeyEnv)); err == nil {
		keyInterval = delay
	}
	if interval > 0 {
		keyInterval = interval
	}

	// Build special key map (lowercase names like packer uses)
	specialMap := map[string]int{
		"bs":         VScanCodes["BACKSPACE"],
		"del":        VScanCodes["DELETE"],
		"down":       VScanCodes["DOWN"],
		"end":        VScanCodes["END"],
		"enter":      VScanCodes["ENTER"],
		"esc":        VScanCodes["ESCAPE"],
		"f1":         VScanCodes["F1"],
		"f2":         VScanCodes["F2"],
		"f3":         VScanCodes["F3"],
		"f4":         VScanCodes["F4"],
		"f5":         VScanCodes["F5"],
		"f6":         VScanCodes["F6"],
		"f7":         VScanCodes["F7"],
		"f8":         VScanCodes["F8"],
		"f9":         VScanCodes["F9"],
		"f10":        VScanCodes["F10"],
		"f11":        VScanCodes["F11"],
		"f12":        VScanCodes["F12"],
		"home":       VScanCodes["HOME"],
		"insert":     VScanCodes["INSERT"],
		"left":       VScanCodes["LEFT"],
		"leftalt":    VScanCodes["LALT"],
		"leftctrl":   VScanCodes["LCTRL"],
		"leftshift":  VScanCodes["LSHIFT"],
		"pagedown":   VScanCodes["PAGEDOWN"],
		"pageup":     VScanCodes["PAGEUP"],
		"return":     VScanCodes["ENTER"],
		"right":      VScanCodes["RIGHT"],
		"rightalt":   VScanCodes["RALT"],
		"rightctrl":  VScanCodes["RCTRL"],
		"rightshift": VScanCodes["RSHIFT"],
		"spacebar":   VScanCodes["SPACE"],
		"tab":        VScanCodes["TAB"],
		"up":         VScanCodes["UP"],
	}

	// Build character to scancode map
	// Maps each character to its PS/2 scan code
	scancodeMap := map[rune]int{
		'1': VScanCodes["1"], '2': VScanCodes["2"], '3': VScanCodes["3"],
		'4': VScanCodes["4"], '5': VScanCodes["5"], '6': VScanCodes["6"],
		'7': VScanCodes["7"], '8': VScanCodes["8"], '9': VScanCodes["9"],
		'0': VScanCodes["0"],
		'!': VScanCodes["1"], '@': VScanCodes["2"], '#': VScanCodes["3"],
		'$': VScanCodes["4"], '%': VScanCodes["5"], '^': VScanCodes["6"],
		'&': VScanCodes["7"], '*': VScanCodes["8"], '(': VScanCodes["9"],
		')': VScanCodes["0"],
		'-': VScanCodes["MINUS"], '_': VScanCodes["MINUS"],
		'=': VScanCodes["EQUALS"], '+': VScanCodes["EQUALS"],
		'q': VScanCodes["Q"], 'w': VScanCodes["W"], 'e': VScanCodes["E"],
		'r': VScanCodes["R"], 't': VScanCodes["T"], 'y': VScanCodes["Y"],
		'u': VScanCodes["U"], 'i': VScanCodes["I"], 'o': VScanCodes["O"],
		'p': VScanCodes["P"],
		'Q': VScanCodes["Q"], 'W': VScanCodes["W"], 'E': VScanCodes["E"],
		'R': VScanCodes["R"], 'T': VScanCodes["T"], 'Y': VScanCodes["Y"],
		'U': VScanCodes["U"], 'I': VScanCodes["I"], 'O': VScanCodes["O"],
		'P': VScanCodes["P"],
		'[': VScanCodes["LBRACKET"], ']': VScanCodes["RBRACKET"],
		'{': VScanCodes["LBRACKET"], '}': VScanCodes["RBRACKET"],
		'a': VScanCodes["A"], 's': VScanCodes["S"], 'd': VScanCodes["D"],
		'f': VScanCodes["F"], 'g': VScanCodes["G"], 'h': VScanCodes["H"],
		'j': VScanCodes["J"], 'k': VScanCodes["K"], 'l': VScanCodes["L"],
		'A': VScanCodes["A"], 'S': VScanCodes["S"], 'D': VScanCodes["D"],
		'F': VScanCodes["F"], 'G': VScanCodes["G"], 'H': VScanCodes["H"],
		'J': VScanCodes["J"], 'K': VScanCodes["K"], 'L': VScanCodes["L"],
		';': VScanCodes["SEMICOLON"], ':': VScanCodes["SEMICOLON"],
		'\'': VScanCodes["QUOTE"], '"': VScanCodes["QUOTE"],
		'`': VScanCodes["BACKTICK"], '~': VScanCodes["BACKTICK"],
		'\\': VScanCodes["BACKSLASH"], '|': VScanCodes["BACKSLASH"],
		'z': VScanCodes["Z"], 'x': VScanCodes["X"], 'c': VScanCodes["C"],
		'v': VScanCodes["V"], 'b': VScanCodes["B"], 'n': VScanCodes["N"],
		'm': VScanCodes["M"],
		'Z': VScanCodes["Z"], 'X': VScanCodes["X"], 'C': VScanCodes["C"],
		'V': VScanCodes["V"], 'B': VScanCodes["B"], 'N': VScanCodes["N"],
		'M': VScanCodes["M"],
		',': VScanCodes["COMMA"], '<': VScanCodes["COMMA"],
		'.': VScanCodes["PERIOD"], '>': VScanCodes["PERIOD"],
		'/': VScanCodes["SLASH"], '?': VScanCodes["SLASH"],
		' ': VScanCodes["SPACE"],
	}

	return &WMKSBootDriver{
		client:      client,
		interval:    keyInterval,
		specialMap:  specialMap,
		scancodeMap: scancodeMap,
	}
}

// SendKey sends a regular character key
func (d *WMKSBootDriver) SendKey(key rune, action bootcommand.KeyAction) error {
	scancode, ok := d.scancodeMap[key]
	if !ok {
		return fmt.Errorf("unknown key: %c", key)
	}

	needsShift := unicode.IsUpper(key) || strings.ContainsRune(shiftedChars, key)

	log.Printf("Sending key '%c' (scancode %d, shift=%v, action=%s)", key, scancode, needsShift, action.String())

	// Handle key down
	if action&(bootcommand.KeyOn|bootcommand.KeyPress) != 0 {
		if needsShift {
			if err := d.client.SendKeyEvent(VScanCodes["LSHIFT"], true); err != nil {
				return err
			}
		}
		if err := d.client.SendKeyEvent(scancode, true); err != nil {
			return err
		}
	}

	// Handle key up
	if action&(bootcommand.KeyOff|bootcommand.KeyPress) != 0 {
		if err := d.client.SendKeyEvent(scancode, false); err != nil {
			return err
		}
		if needsShift {
			if err := d.client.SendKeyEvent(VScanCodes["LSHIFT"], false); err != nil {
				return err
			}
		}
	}

	time.Sleep(d.interval)
	return nil
}

// SendSpecial sends a special key (like enter, esc, f1, etc.)
func (d *WMKSBootDriver) SendSpecial(special string, action bootcommand.KeyAction) error {
	scancode, ok := d.specialMap[special]
	if !ok {
		return fmt.Errorf("unknown special key: %s", special)
	}

	log.Printf("Sending special key '%s' (scancode %d, action=%s)", special, scancode, action.String())

	// Handle key down
	if action&(bootcommand.KeyOn|bootcommand.KeyPress) != 0 {
		if err := d.client.SendKeyEvent(scancode, true); err != nil {
			return err
		}
	}

	// Handle key up
	if action&(bootcommand.KeyOff|bootcommand.KeyPress) != 0 {
		if err := d.client.SendKeyEvent(scancode, false); err != nil {
			return err
		}
	}

	time.Sleep(d.interval)
	return nil
}

// Flush sends any buffered scancodes - WMKS sends immediately so this is a no-op
func (d *WMKSBootDriver) Flush() error {
	return nil
}
