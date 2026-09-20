package geofile

import (
	"fmt"
	"net/url"
	"strings"
)

const (
	XrayGeoURLTemplate     = "https://github.com/Loyalsoldier/v2ray-rules-dat/releases/latest/download/%s.dat"
	SingRuleSetURLTemplate = "https://raw.githubusercontent.com/2dust/sing-box-rules/rule-set-%s/%s.srs"
	geoFilePrefix          = "geofile:"
)

// Kind identifies which family of geo data a GeoFileSpec represents.
type Kind string

const (
	KindGeoIP   Kind = "geoip"
	KindGeoSite Kind = "geosite"
)

// Spec is the normalized form of a geofile declaration.
//
// A spec without Name refers to an Xray aggregate .dat file. A named spec
// also identifies the corresponding Sing-box rule-set tag and file.
type Spec struct {
	Kind Kind
	Name string
}

// Parse converts a declaration such as
// geofile:geosite-category-cryptocurrency into a normalized spec.
func Parse(value string) (Spec, error) {
	value = strings.TrimSpace(value)
	if len(value) < len(geoFilePrefix) || !strings.EqualFold(value[:len(geoFilePrefix)], geoFilePrefix) {
		return Spec{}, fmt.Errorf("invalid geofile declaration %q: must start with %q", value, geoFilePrefix)
	}

	payload := strings.ToLower(strings.TrimSpace(value[len(geoFilePrefix):]))
	switch payload {
	case string(KindGeoIP):
		return Spec{Kind: KindGeoIP}, nil
	case string(KindGeoSite):
		return Spec{Kind: KindGeoSite}, nil
	}

	for _, kind := range []Kind{KindGeoIP, KindGeoSite} {
		prefix := string(kind) + "-"
		if !strings.HasPrefix(payload, prefix) {
			continue
		}

		name := strings.TrimPrefix(payload, prefix)
		if err := validateName(name); err != nil {
			return Spec{}, fmt.Errorf("invalid geofile declaration %q: %w", value, err)
		}
		return Spec{Kind: kind, Name: name}, nil
	}

	return Spec{}, fmt.Errorf("invalid geofile declaration %q: expected geosite[-name] or geoip[-name]", value)
}

// Normalize parses declarations, removes duplicates, and preserves the first
// occurrence of each canonical declaration.
func Normalize(values []string) ([]Spec, error) {
	specs := make([]Spec, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for index, value := range values {
		spec, err := Parse(value)
		if err != nil {
			return nil, fmt.Errorf("GeoFiles[%d]: %w", index, err)
		}
		key := spec.String()
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		specs = append(specs, spec)
	}
	return specs, nil
}

// String returns the canonical V2bX declaration.
func (s Spec) String() string {
	if s.Name == "" {
		return geoFilePrefix + string(s.Kind)
	}
	return geoFilePrefix + string(s.Kind) + "-" + s.Name
}

// XrayFileName returns the aggregate asset required by Xray.
func (s Spec) XrayFileName() string {
	return string(s.Kind) + ".dat"
}

// XrayURL returns the v2rayN-compatible URL for the aggregate Xray asset.
func (s Spec) XrayURL() string {
	return fmt.Sprintf(XrayGeoURLTemplate, s.Kind)
}

// RuleSetTag returns the Sing-box rule-set tag for a named spec. Aggregate
// specs do not have a corresponding per-category rule-set.
func (s Spec) RuleSetTag() string {
	if s.Name == "" {
		return ""
	}
	return string(s.Kind) + "-" + s.Name
}

// RuleSetURL returns the v2rayN-compatible Sing-box rule-set URL. The second
// return value is false for aggregate specs such as geofile:geosite.
func (s Spec) RuleSetURL() (string, bool) {
	tag := s.RuleSetTag()
	if tag == "" {
		return "", false
	}
	return fmt.Sprintf(SingRuleSetURLTemplate, s.Kind, url.PathEscape(tag)), true
}

func validateName(name string) error {
	if name == "" {
		return fmt.Errorf("rule-set name is empty")
	}
	if strings.Contains(name, "..") {
		return fmt.Errorf("rule-set name must not contain %q", "..")
	}

	for index, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			if index == 0 && (r == '-' || r == '_' || r == '.') {
				return fmt.Errorf("rule-set name must start with a letter or digit")
			}
			continue
		}
		return fmt.Errorf("rule-set name contains unsupported character %q", r)
	}
	return nil
}
