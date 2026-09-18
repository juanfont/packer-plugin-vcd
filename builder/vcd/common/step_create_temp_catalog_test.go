package common

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/vmware/go-vcloud-director/v3/govcd"
	"github.com/vmware/go-vcloud-director/v3/types/v56"
)

// fakeCatalogStore scripts the answers of successive GetCatalog calls.
type fakeCatalogStore struct {
	getErrs   []error // one per GetCatalog call; nil means found
	createErr error
	gets      int
	creates   int
}

func (f *fakeCatalogStore) GetCatalog(string) (*govcd.Catalog, error) {
	err := f.getErrs[f.gets]
	f.gets++
	if err != nil {
		return nil, err
	}
	return &govcd.Catalog{}, nil
}

func (f *fakeCatalogStore) CreateCatalogWithStorageProfile(string, string, *types.Reference) (*govcd.AdminCatalog, error) {
	f.creates++
	return nil, f.createErr
}

var (
	errNotFound = fmt.Errorf("error getting catalog x: %w", govcd.ErrorEntityNotFound)
	noProfile   = func() (*types.Reference, error) { return nil, nil }
)

func TestGetOrCreateCatalog(t *testing.T) {
	tests := []struct {
		name        string
		store       *fakeCatalogStore
		wantCreated bool
		wantCreates int
		wantErr     string
	}{
		{"exists", &fakeCatalogStore{getErrs: []error{nil}}, false, 0, ""},
		{"missing, created", &fakeCatalogStore{getErrs: []error{errNotFound, nil}}, true, 1, ""},
		{"missing, a parallel build won the create race",
			&fakeCatalogStore{getErrs: []error{errNotFound, nil}, createErr: errors.New("already exists")}, false, 1, ""},
		{"missing, create failed for real",
			&fakeCatalogStore{getErrs: []error{errNotFound, errNotFound}, createErr: errors.New("no rights")}, false, 1, "no rights"},
		{"lookup failed for another reason: do not create",
			&fakeCatalogStore{getErrs: []error{errors.New("401 unauthorized")}}, false, 0, "401 unauthorized"},
	}
	for _, tt := range tests {
		catalog, created, err := getOrCreateCatalog(tt.store, "x", noProfile)
		if tt.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("%s: err = %v, want it to contain %q", tt.name, err, tt.wantErr)
			}
		} else if err != nil || catalog == nil {
			t.Errorf("%s: unexpected result: catalog=%v err=%v", tt.name, catalog, err)
		}
		if created != tt.wantCreated || tt.store.creates != tt.wantCreates {
			t.Errorf("%s: created=%v creates=%d, want %v/%d", tt.name, created, tt.store.creates, tt.wantCreated, tt.wantCreates)
		}
	}
}

func TestGetOrCreateCatalogStorageProfileError(t *testing.T) {
	store := &fakeCatalogStore{getErrs: []error{errNotFound}}
	_, _, err := getOrCreateCatalog(store, "x", func() (*types.Reference, error) { return nil, errors.New("no such profile") })
	if err == nil || store.creates != 0 {
		t.Errorf("err=%v creates=%d, want an error and no create", err, store.creates)
	}
}
