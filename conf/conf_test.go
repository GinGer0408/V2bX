package conf

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConf_LoadFromPath(t *testing.T) {
	c := New()
	t.Log(c.LoadFromPath("../example/config.json"), c.NodeConfig)
}

func TestConf_GeoFiles(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	config := []byte(`{
  "GeoFiles": [
    "geofile:geosite-category-cryptocurrency",
    "geofile:geoip-cn"
  ]
}`)
	if err := os.WriteFile(configPath, config, 0600); err != nil {
		t.Fatal(err)
	}

	c := New()
	if err := c.LoadFromPath(configPath); err != nil {
		t.Fatal(err)
	}
	specs, err := c.GeoFileSpecs()
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 2 {
		t.Fatalf("got %d geofile specs, want 2", len(specs))
	}
	if got := specs[0].String(); got != "geofile:geosite-category-cryptocurrency" {
		t.Fatalf("first spec = %q", got)
	}
}

func TestConf_Watch(t *testing.T) {
	c := New()
	t.Log(c.Watch("./1.json", "", "", func() {}))
	select {}
}
