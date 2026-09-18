package common

import "testing"

func TestShouldUseISOCache(t *testing.T) {
	tests := []struct {
		name                               string
		cacheISO, isoModified, tempCatalog bool
		want                               bool
	}{
		{"persistent catalog, pristine ISO", true, false, false, true},
		{"modified ISO is unique per build", true, true, false, false},
		{"temp catalog is always empty", true, false, true, false},
		{"caching disabled", false, false, false, false},
		{"cache_overwrite clears CacheISO", false, true, false, false},
	}
	for _, tt := range tests {
		if got := shouldUseISOCache(tt.cacheISO, tt.isoModified, tt.tempCatalog); got != tt.want {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}
