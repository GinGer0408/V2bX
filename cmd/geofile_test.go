package cmd

import (
	"testing"

	"github.com/InazumaV/V2bX/conf"
	"github.com/InazumaV/V2bX/geofile"
)

func TestPrepareGeoConfigPropagatesSpecsToCores(t *testing.T) {
	c := conf.New()
	c.GeoFiles = []string{"geofile:geosite-category-cryptocurrency"}
	c.CoresConfig = []conf.CoreConfig{{Type: "sing"}}
	state := &geoRuntimeState{}

	if _, err := prepareGeoConfig(c, geofile.NewManager(nil), state); err != nil {
		t.Fatal(err)
	}
	if len(c.CoresConfig[0].GeoFiles) != 1 {
		t.Fatalf("core got %d GeoFiles, want 1", len(c.CoresConfig[0].GeoFiles))
	}
	if got := c.CoresConfig[0].GeoFiles[0].String(); got != "geofile:geosite-category-cryptocurrency" {
		t.Fatalf("core GeoFile = %q", got)
	}
	specs, _ := state.snapshot()
	if len(specs) != 1 {
		t.Fatalf("runtime state got %d specs, want 1", len(specs))
	}
}
