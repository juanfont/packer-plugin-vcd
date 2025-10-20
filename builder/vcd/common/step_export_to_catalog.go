package common

import "fmt"

//go:generate packer-sdc struct-markdown
//go:generate packer-sdc mapstructure-to-hcl2 -type ExportToCatalogConfig

type ExportToCatalogConfig struct {
	// The name of the catalog to export the virtual machine to.
	Name string `mapstructure:"name"`
}

func (c *ExportToCatalogConfig) Prepare(lc *LocationConfig) []error {
	var errs []error

	if c.Name == "" {
		errs = append(errs, fmt.Errorf("'name' is required"))
	}

	return errs
}
