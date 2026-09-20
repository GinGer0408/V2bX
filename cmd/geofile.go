package cmd

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/InazumaV/V2bX/conf"
	"github.com/InazumaV/V2bX/geofile"
	log "github.com/sirupsen/logrus"
)

type geoRuntimeState struct {
	mu      sync.RWMutex
	specs   []geofile.Spec
	targets []geofile.XrayTarget
}

func (s *geoRuntimeState) set(specs []geofile.Spec, targets []geofile.XrayTarget) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.specs = append([]geofile.Spec(nil), specs...)
	s.targets = append([]geofile.XrayTarget(nil), targets...)
}

func (s *geoRuntimeState) snapshot() ([]geofile.Spec, []geofile.XrayTarget) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]geofile.Spec(nil), s.specs...), append([]geofile.XrayTarget(nil), s.targets...)
}

func prepareGeoConfig(c *conf.Conf, manager *geofile.Manager, state *geoRuntimeState) (geofile.UpdateReport, error) {
	specs, err := c.GeoFileSpecs()
	if err != nil {
		return geofile.UpdateReport{}, err
	}
	targets := xrayTargets(c)
	report, err := manager.UpdateXray(context.Background(), specs, targets)
	if err != nil {
		return report, err
	}

	for i := range c.CoresConfig {
		c.CoresConfig[i].GeoFiles = append([]geofile.Spec(nil), specs...)
	}
	state.set(specs, targets)
	logGeoReport(report)
	return report, nil
}

func xrayTargets(c *conf.Conf) []geofile.XrayTarget {
	var targets []geofile.XrayTarget
	for _, coreConfig := range c.CoresConfig {
		if !strings.EqualFold(coreConfig.Type, "xray") || coreConfig.XrayConfig == nil {
			continue
		}
		assetPath := coreConfig.XrayConfig.AssetPath
		if strings.TrimSpace(assetPath) == "" {
			assetPath = "/etc/V2bX/"
		}
		targets = append(targets, geofile.XrayTarget{
			AssetPath:       assetPath,
			DNSConfigPath:   coreConfig.XrayConfig.DnsConfigPath,
			RouteConfigPath: coreConfig.XrayConfig.RouteConfigPath,
		})
	}
	return targets
}

func logGeoReport(report geofile.UpdateReport) {
	for _, result := range report.Results {
		if result.Err != nil {
			log.WithError(result.Err).WithField("path", result.Path).Warn("GeoFile update failed; keeping the existing asset")
			continue
		}
		if result.Changed {
			log.WithFields(log.Fields{
				"kind": result.Kind,
				"path": result.Path,
				"size": result.Size,
			}).Info("GeoFile updated")
		}
	}
}

func startGeoUpdater(ctx context.Context, manager *geofile.Manager, state *geoRuntimeState, reload func()) {
	go func() {
		ticker := time.NewTicker(geofile.DefaultUpdateInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				specs, targets := state.snapshot()
				updateCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
				report, err := manager.UpdateXray(updateCtx, specs, targets)
				cancel()
				if err != nil {
					log.WithError(err).Warn("periodic GeoFile update failed")
				}
				logGeoReport(report)
				if err == nil && report.Changed() {
					reload()
				}
			}
		}
	}()
}

func geoReloadError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("prepare GeoFiles: %w", err)
}
