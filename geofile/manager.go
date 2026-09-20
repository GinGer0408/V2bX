package geofile

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	DefaultUpdateInterval = 24 * time.Hour
	defaultHTTPTimeout    = 2 * time.Minute
	defaultMinAssetBytes  = 1024
	defaultMaxAssetBytes  = 128 << 20
)

// XrayTarget describes one Xray asset directory and its optional route file.
type XrayTarget struct {
	AssetPath       string
	DNSConfigPath   string
	RouteConfigPath string
}

// UpdateResult records one Xray asset update attempt.
type UpdateResult struct {
	Kind    Kind
	Path    string
	URL     string
	Changed bool
	Size    int64
	Err     error
}

// UpdateReport describes all update attempts. A non-nil Err in a result is a
// recoverable warning when the existing asset is still available.
type UpdateReport struct {
	Results []UpdateResult
}

// Changed reports whether at least one asset was replaced.
func (r UpdateReport) Changed() bool {
	for _, result := range r.Results {
		if result.Changed {
			return true
		}
	}
	return false
}

// Manager owns the network and filesystem side of GeoFile updates. The
// mutex prevents a config reload and the periodic updater from replacing the
// same asset concurrently.
type Manager struct {
	Client       *http.Client
	MinAssetSize int64
	MaxAssetSize int64

	mu sync.Mutex
}

func NewManager(client *http.Client) *Manager {
	if client == nil {
		client = &http.Client{Timeout: defaultHTTPTimeout}
	}
	return &Manager{
		Client:       client,
		MinAssetSize: defaultMinAssetBytes,
		MaxAssetSize: defaultMaxAssetBytes,
	}
}

// UpdateXray updates the aggregate geoip.dat/geosite.dat files needed by the
// configured Xray targets. The required kinds come from explicit GeoFiles
// declarations and from geoip:/geosite: references in route files.
func (m *Manager) UpdateXray(ctx context.Context, specs []Spec, targets []XrayTarget) (UpdateReport, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	report := UpdateReport{}
	if len(targets) == 0 {
		return report, nil
	}

	for _, target := range targets {
		assetPath := strings.TrimSpace(target.AssetPath)
		if assetPath == "" {
			assetPath = "/etc/V2bX/"
		}
		assetPath = filepath.Clean(assetPath)

		kinds := make(map[Kind]struct{})
		for _, spec := range specs {
			kinds[spec.Kind] = struct{}{}
		}
		for kind := range kindsFromRouteFile(target.RouteConfigPath) {
			kinds[kind] = struct{}{}
		}
		for kind := range kindsFromRouteFile(target.DNSConfigPath) {
			kinds[kind] = struct{}{}
		}

		for _, kind := range []Kind{KindGeoIP, KindGeoSite} {
			if _, ok := kinds[kind]; !ok {
				continue
			}
			spec := Spec{Kind: kind}
			path := filepath.Join(assetPath, spec.XrayFileName())
			changed, size, err := m.updateAsset(ctx, path, spec.XrayURL())
			result := UpdateResult{
				Kind:    kind,
				Path:    path,
				URL:     spec.XrayURL(),
				Changed: changed,
				Size:    size,
				Err:     err,
			}
			report.Results = append(report.Results, result)
			if err == nil {
				continue
			}

			if existingAssetIsUsable(path, m.MinAssetSize) {
				// Keep serving with the last known-good asset. The caller can
				// log this as a warning and retry on the next interval.
				continue
			}
			return report, fmt.Errorf("update Xray %s: %w", path, err)
		}
	}

	return report, nil
}

func (m *Manager) updateAsset(ctx context.Context, path, sourceURL string) (bool, int64, error) {
	if m.Client == nil {
		return false, 0, errors.New("geofile HTTP client is nil")
	}
	minAssetSize := m.MinAssetSize
	if minAssetSize <= 0 {
		minAssetSize = defaultMinAssetBytes
	}
	maxAssetSize := m.MaxAssetSize
	if maxAssetSize <= 0 {
		maxAssetSize = defaultMaxAssetBytes
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return false, 0, fmt.Errorf("create asset directory: %w", err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return false, 0, fmt.Errorf("create request: %w", err)
	}
	response, err := m.Client.Do(request)
	if err != nil {
		return false, 0, fmt.Errorf("download %s: %w", sourceURL, err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return false, 0, fmt.Errorf("download %s: unexpected HTTP status %s", sourceURL, response.Status)
	}

	temporary, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-")
	if err != nil {
		return false, 0, fmt.Errorf("create temporary asset: %w", err)
	}
	temporaryPath := temporary.Name()
	keepTemporary := false
	defer func() {
		_ = temporary.Close()
		if !keepTemporary {
			_ = os.Remove(temporaryPath)
		}
	}()

	hash := sha256.New()
	reader := io.LimitReader(response.Body, maxAssetSize+1)
	size, err := io.Copy(io.MultiWriter(temporary, hash), reader)
	if err != nil {
		return false, 0, fmt.Errorf("save downloaded asset: %w", err)
	}
	if size > maxAssetSize {
		return false, size, fmt.Errorf("downloaded asset is larger than %d bytes", maxAssetSize)
	}
	if size < minAssetSize {
		return false, size, fmt.Errorf("downloaded asset is only %d bytes", size)
	}

	if err := temporary.Sync(); err != nil {
		return false, size, fmt.Errorf("sync temporary asset: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return false, size, fmt.Errorf("close temporary asset: %w", err)
	}
	if err := validateDownloadedAsset(temporaryPath); err != nil {
		return false, size, err
	}

	newHash := hash.Sum(nil)
	if oldHash, oldSize, err := hashFile(path); err == nil && oldSize == size && bytes.Equal(oldHash, newHash) {
		return false, size, nil
	}
	if err := os.Chmod(temporaryPath, 0644); err != nil {
		return false, size, fmt.Errorf("set asset permissions: %w", err)
	}
	if err := replaceFile(temporaryPath, path); err != nil {
		return false, size, fmt.Errorf("replace asset: %w", err)
	}
	keepTemporary = true
	return true, size, nil
}

func validateDownloadedAsset(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open downloaded asset for validation: %w", err)
	}
	defer file.Close()

	prefix := make([]byte, 512)
	read, err := file.Read(prefix)
	if err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("read downloaded asset for validation: %w", err)
	}
	text := strings.ToLower(strings.TrimSpace(string(prefix[:read])))
	if strings.HasPrefix(text, "<") || strings.HasPrefix(text, "{") || strings.HasPrefix(text, "[") {
		return fmt.Errorf("downloaded asset appears to be an error page")
	}
	return nil
}

func existingAssetIsUsable(path string, minSize int64) bool {
	if minSize <= 0 {
		minSize = defaultMinAssetBytes
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() < minSize {
		return false
	}
	return validateDownloadedAsset(path) == nil
}

func hashFile(path string) ([]byte, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer file.Close()

	hash := sha256.New()
	size, err := io.Copy(hash, file)
	if err != nil {
		return nil, 0, err
	}
	return hash.Sum(nil), size, nil
}

func replaceFile(source, destination string) error {
	if err := os.Rename(source, destination); err == nil {
		return nil
	}

	// os.Rename replaces an existing file atomically on Unix. Windows does
	// not allow that operation, so retain a rollback copy for that platform.
	backup := destination + ".bak"
	_ = os.Remove(backup)
	if err := os.Rename(destination, backup); err != nil {
		return err
	}
	if err := os.Rename(source, destination); err != nil {
		_ = os.Rename(backup, destination)
		return err
	}
	_ = os.Remove(backup)
	return nil
}

func kindsFromRouteFile(path string) map[Kind]struct{} {
	result := make(map[Kind]struct{})
	if strings.TrimSpace(path) == "" {
		return result
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return result
	}
	lower := strings.ToLower(string(data))
	if strings.Contains(lower, "geoip:") {
		result[KindGeoIP] = struct{}{}
	}
	if strings.Contains(lower, "geosite:") {
		result[KindGeoSite] = struct{}{}
	}
	return result
}
