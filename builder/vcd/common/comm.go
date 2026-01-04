package common

import (
	"github.com/hashicorp/packer-plugin-sdk/multistep"
)

// CommHost returns a function that retrieves the communicator host IP
// from the state bag. This is used by communicator.StepConnect to know
// where to connect.
func CommHost(configuredHost string) func(multistep.StateBag) (string, error) {
	return func(state multistep.StateBag) (string, error) {
		// If host is configured explicitly, use that
		if configuredHost != "" {
			return configuredHost, nil
		}

		// Otherwise get the IP from StepWaitForIP
		if ip, ok := state.GetOk("ip"); ok {
			return ip.(string), nil
		}

		return "", nil
	}
}
