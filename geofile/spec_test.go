package geofile

import (
	"reflect"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		want       Spec
		wantString string
		wantURL    string
		wantOK     bool
	}{
		{
			name:       "named geosite",
			input:      "geofile:geosite-category-cryptocurrency",
			want:       Spec{Kind: KindGeoSite, Name: "category-cryptocurrency"},
			wantString: "geofile:geosite-category-cryptocurrency",
			wantURL:    "https://raw.githubusercontent.com/2dust/sing-box-rules/rule-set-geosite/geosite-category-cryptocurrency.srs",
			wantOK:     true,
		},
		{
			name:       "named geoip",
			input:      " GEOFILE:GEOIP-CN ",
			want:       Spec{Kind: KindGeoIP, Name: "cn"},
			wantString: "geofile:geoip-cn",
			wantURL:    "https://raw.githubusercontent.com/2dust/sing-box-rules/rule-set-geoip/geoip-cn.srs",
			wantOK:     true,
		},
		{
			name:       "aggregate geosite",
			input:      "geofile:geosite",
			want:       Spec{Kind: KindGeoSite},
			wantString: "geofile:geosite",
			wantOK:     true,
		},
		{
			name:   "missing prefix",
			input:  "geosite:cn",
			wantOK: false,
		},
		{
			name:   "unsupported kind",
			input:  "geofile:foo-cn",
			wantOK: false,
		},
		{
			name:   "path traversal",
			input:  "geofile:geosite-../private",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.input)
			if (err == nil) != tt.wantOK {
				t.Fatalf("Parse() error = %v, wantOK %v", err, tt.wantOK)
			}
			if !tt.wantOK {
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("Parse() = %#v, want %#v", got, tt.want)
			}
			if got.String() != tt.wantString {
				t.Fatalf("String() = %q, want %q", got.String(), tt.wantString)
			}
			if got.XrayFileName() != string(got.Kind)+".dat" {
				t.Fatalf("XrayFileName() = %q", got.XrayFileName())
			}
			if got.XrayURL() == "" {
				t.Fatal("XrayURL() is empty")
			}
			if tt.wantURL == "" {
				if _, ok := got.RuleSetURL(); ok {
					t.Fatal("aggregate spec unexpectedly has a rule-set URL")
				}
				return
			}
			url, ok := got.RuleSetURL()
			if !ok || url != tt.wantURL {
				t.Fatalf("RuleSetURL() = %q, %v; want %q, true", url, ok, tt.wantURL)
			}
		})
	}
}

func TestNormalizeDeduplicatesAndPreservesOrder(t *testing.T) {
	got, err := Normalize([]string{
		"geofile:geosite-category-cryptocurrency",
		"geofile:geoip-cn",
		" GEOFILE:GEOSITE-CATEGORY-CRYPTOCURRENCY ",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []Spec{
		{Kind: KindGeoSite, Name: "category-cryptocurrency"},
		{Kind: KindGeoIP, Name: "cn"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Normalize() = %#v, want %#v", got, want)
	}
}
