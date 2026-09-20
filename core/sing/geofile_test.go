package sing

import (
	"testing"

	"github.com/InazumaV/V2bX/geofile"
	"github.com/sagernet/sing-box/option"
)

func TestApplyGeoFiles(t *testing.T) {
	spec, err := geofile.Parse("geofile:geosite-category-cryptocurrency")
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := geofile.Parse("geofile:geoip-cn")
	if err != nil {
		t.Fatal(err)
	}

	options := option.Options{
		Route: &option.RouteOptions{
			RuleSet: []option.RuleSet{{Tag: "geoip-cn"}},
		},
	}
	applyGeoFiles(&options, []geofile.Spec{spec, duplicate})

	if options.Route == nil || len(options.Route.RuleSet) != 2 {
		t.Fatalf("got %d rule-sets, want 2", len(options.Route.RuleSet))
	}
	generated := options.Route.RuleSet[1]
	if generated.Type != "remote" {
		t.Fatalf("generated rule-set type = %q", generated.Type)
	}
	if generated.Tag != "geosite-category-cryptocurrency" {
		t.Fatalf("generated rule-set tag = %q", generated.Tag)
	}
	if generated.Format != "binary" {
		t.Fatalf("generated rule-set format = %q", generated.Format)
	}
	if generated.RemoteOptions.URL != "https://raw.githubusercontent.com/2dust/sing-box-rules/rule-set-geosite/geosite-category-cryptocurrency.srs" {
		t.Fatalf("generated rule-set URL = %q", generated.RemoteOptions.URL)
	}
	if generated.RemoteOptions.DownloadDetour != "direct" {
		t.Fatalf("generated download detour = %q", generated.RemoteOptions.DownloadDetour)
	}
	if generated.RemoteOptions.UpdateInterval.Build().Hours() != 24 {
		t.Fatalf("generated update interval = %s", generated.RemoteOptions.UpdateInterval.Build())
	}
}
