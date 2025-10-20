package common

import (
	"context"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
)

type RemoveNetworkAdapterConfig struct {
	// Remove all network adapters from the virtual machine image. Defaults to `false`.
	RemoveNetworkAdapter bool `mapstructure:"remove_network_adapter"`
}

type StepRemoveNetworkAdapter struct {
	Config *RemoveNetworkAdapterConfig
}

func (s *StepRemoveNetworkAdapter) Run(_ context.Context, state multistep.StateBag) multistep.StepAction {
	if !s.Config.RemoveNetworkAdapter {
		return multistep.ActionContinue
	}

	// TODO(juan): Implement this
	// https://github.com/hashicorp/packer-plugin-vsphere/blob/main/builder/vsphere/common/step_remove_network_adapter.go#L18

	return multistep.ActionContinue
}
