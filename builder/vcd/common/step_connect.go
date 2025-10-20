package common

import (
	"context"
	"fmt"
	"log"
	"reflect"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/juanfont/packer-plugin-vcd/builder/vcd/driver"
)

type ConnectConfig struct {
	// The fully qualified domain name or IP address of the vCD Server instance.
	Host string `mapstructure:"host"`
	// The username to authenticate with the vCD Server instance.
	Org string `mapstructure:"org"`
	// The username to authenticate with the vCD Server instance.
	Username string `mapstructure:"username"`
	// The password to authenticate with the vCD Server instance.
	Password string `mapstructure:"password"`
	// The token to authenticate with the vCenter Server instance.
	Token string `mapstructure:"token"`

	// Do not validate the certificate of the vCD Server instance.
	// Defaults to `false`.
	//
	// -> **Note:** This option is beneficial in scenarios where the certificate
	// is self-signed or does not meet standard validation criteria.
	InsecureConnection bool `mapstructure:"insecure_connection"`
}

func (c *ConnectConfig) Prepare() []error {
	var errs []error

	if c.Host == "" {
		errs = append(errs, fmt.Errorf("'host' is required"))
	}
	if c.Token == "" {
		if c.Username == "" {
			errs = append(errs, fmt.Errorf("'username' is required if 'token' is not provided"))
		}
		if c.Password == "" {
			errs = append(errs, fmt.Errorf("'password' is required if 'token' is not provided"))
		}
	}

	if c.Token == "" && c.Username == "" {
		errs = append(errs, fmt.Errorf("'username' or 'token' is required"))
	}

	if c.Org == "" {
		errs = append(errs, fmt.Errorf("'org' is required"))
	}

	return errs
}

type StepConnect struct {
	Config *ConnectConfig
}

func (s *StepConnect) Run(_ context.Context, state multistep.StateBag) multistep.StepAction {
	d, err := driver.NewDriver(&driver.ConnectConfig{
		Host:               s.Config.Host,
		Org:                s.Config.Org,
		Username:           s.Config.Username,
		Password:           s.Config.Password,
		Token:              s.Config.Token,
		InsecureConnection: s.Config.InsecureConnection,
	})
	if err != nil {
		state.Put("error", err)
		return multistep.ActionHalt
	}
	state.Put("driver", d)

	return multistep.ActionContinue
}

func (s *StepConnect) Cleanup(state multistep.StateBag) {
	ui := state.Get("ui").(packersdk.Ui)
	d, ok := state.GetOk("driver")
	if !ok {
		log.Printf("[INFO] No driver in state; nothing to cleanup.")
		return
	}

	driver, ok := d.(driver.Driver)
	if !ok {
		log.Printf("[ERROR] The object stored in the state under 'driver' key is of type '%s', not 'driver.Driver'. This could indicate a problem with the state initialization or management.", reflect.TypeOf(d))
		return
	}

	ui.Say("Closing sessions...")

	err := driver.Cleanup()
	if err != nil {
		log.Printf("[WARN] Failed to close VCD client session. The session may already be closed: %s", err.Error())
	}
}
