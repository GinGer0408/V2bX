package geofile

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManagerUpdateXrayIsAtomicAndKeepsLastGoodAsset(t *testing.T) {
	good := []byte(strings.Repeat("geo-data-", 256))
	responseBody := good
	responseStatus := http.StatusOK
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(responseStatus)
		_, _ = w.Write(responseBody)
	}))
	defer server.Close()

	manager := NewManager(server.Client())
	manager.MinAssetSize = 64
	assetDir := t.TempDir()
	targets := []XrayTarget{{AssetPath: assetDir}}
	specs := []Spec{{Kind: KindGeoSite}}

	// Replace the source URL for this test without changing the production
	// mapping by using a client transport that redirects the official URL.
	manager.Client.Transport = redirectTransport{url: server.URL}
	report, err := manager.UpdateXray(context.Background(), specs, targets)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Changed() {
		t.Fatal("first update did not replace the asset")
	}
	path := filepath.Join(assetDir, "geosite.dat")
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(good) {
		t.Fatal("first update wrote unexpected data")
	}

	report, err = manager.UpdateXray(context.Background(), specs, targets)
	if err != nil {
		t.Fatal(err)
	}
	if report.Changed() {
		t.Fatal("unchanged asset was replaced")
	}

	responseStatus = http.StatusInternalServerError
	responseBody = []byte("server error")
	report, err = manager.UpdateXray(context.Background(), specs, targets)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Results) != 1 || report.Results[0].Err == nil {
		t.Fatal("HTTP failure was not recorded as a recoverable warning")
	}
	got, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(good) {
		t.Fatal("HTTP failure damaged the last good asset")
	}

	responseStatus = http.StatusOK
	responseBody = []byte("short")
	report, err = manager.UpdateXray(context.Background(), specs, targets)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Results) != 1 || report.Results[0].Err == nil {
		t.Fatal("short response was not rejected")
	}
	got, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(good) {
		t.Fatal("short response damaged the last good asset")
	}
}

func TestKindsFromRouteFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "route.json")
	if err := os.WriteFile(path, []byte(`{
  "domain": ["geosite:netflix"],
  "ip": ["geoip:private"]
}`), 0600); err != nil {
		t.Fatal(err)
	}
	got := kindsFromRouteFile(path)
	if _, ok := got[KindGeoSite]; !ok {
		t.Fatal("geosite reference was not discovered")
	}
	if _, ok := got[KindGeoIP]; !ok {
		t.Fatal("geoip reference was not discovered")
	}
}

type redirectTransport struct {
	url string
}

func (t redirectTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	clone.URL, _ = request.URL.Parse(t.url)
	return http.DefaultTransport.RoundTrip(clone)
}
