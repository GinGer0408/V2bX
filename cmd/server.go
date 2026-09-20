package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"

	"github.com/InazumaV/V2bX/conf"
	vCore "github.com/InazumaV/V2bX/core"
	"github.com/InazumaV/V2bX/geofile"
	"github.com/InazumaV/V2bX/limiter"
	"github.com/InazumaV/V2bX/node"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	config string
	watch  bool
)

var serverCommand = cobra.Command{
	Use:   "server",
	Short: "Run V2bX server",
	Run:   serverHandle,
	Args:  cobra.NoArgs,
}

func init() {
	serverCommand.PersistentFlags().
		StringVarP(&config, "config", "c",
			"/etc/V2bX/config.json", "config file path")
	serverCommand.PersistentFlags().
		BoolVarP(&watch, "watch", "w",
			true, "watch file path change")
	command.AddCommand(&serverCommand)
}

func serverHandle(_ *cobra.Command, _ []string) {
	showVersion()
	c := conf.New()
	err := c.LoadFromPath(config)
	if err != nil {
		log.WithField("err", err).Error("Load config file failed")
		return
	}
	switch c.LogConfig.Level {
	case "debug":
		log.SetLevel(log.DebugLevel)
	case "info":
		log.SetLevel(log.InfoLevel)
	case "warn":
		log.SetLevel(log.WarnLevel)
	case "error":
		log.SetLevel(log.ErrorLevel)
	}
	if c.LogConfig.Output != "" {
		f, err := os.OpenFile(c.LogConfig.Output, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			log.WithField("err", err).Error("Open log file failed, using stdout instead")
		} else {
			log.SetOutput(f)
		}
	}
	limiter.Init()
	log.Info("Start V2bX...")
	geoManager := geofile.NewManager(nil)
	geoState := &geoRuntimeState{}
	if _, err = prepareGeoConfig(c, geoManager, geoState); err != nil {
		log.WithField("err", geoReloadError(err)).Error("Prepare GeoFiles failed")
		return
	}

	var vc vCore.Core
	nodes := node.New()
	var lifecycleMu sync.Mutex
	startCore := func() error {
		newCore, newCoreErr := vCore.NewCore(c.CoresConfig)
		if newCoreErr != nil {
			return fmt.Errorf("new core failed: %w", newCoreErr)
		}
		if newCoreErr = newCore.Start(); newCoreErr != nil {
			_ = newCore.Close()
			return fmt.Errorf("start core failed: %w", newCoreErr)
		}
		if newCoreErr = nodes.Start(c.NodeConfig, newCore); newCoreErr != nil {
			_ = newCore.Close()
			return fmt.Errorf("run nodes failed: %w", newCoreErr)
		}
		vc = newCore
		log.Info("Core ", vc.Type(), " started")
		log.Info("Nodes started")
		return nil
	}
	if err = startCore(); err != nil {
		log.WithField("err", err).Error("Start V2bX failed")
		return
	}

	runtimeContext, cancelRuntime := context.WithCancel(context.Background())
	defer cancelRuntime()
	defer func() {
		lifecycleMu.Lock()
		defer lifecycleMu.Unlock()
		nodes.Close()
		if vc != nil {
			if closeErr := vc.Close(); closeErr != nil {
				log.WithField("err", closeErr).Error("Close core failed")
			}
		}
	}()

	xdns := os.Getenv("XRAY_DNS_PATH")
	sdns := os.Getenv("SING_DNS_PATH")
	reload := func(reason string, refreshGeo bool) {
		lifecycleMu.Lock()
		defer lifecycleMu.Unlock()

		if refreshGeo {
			if _, refreshErr := prepareGeoConfig(c, geoManager, geoState); refreshErr != nil {
				log.WithField("err", geoReloadError(refreshErr)).Error("Prepare GeoFiles for reload failed")
				return
			}
		}

		nodes.Close()
		if vc != nil {
			if closeErr := vc.Close(); closeErr != nil {
				log.WithField("err", closeErr).Error("Close core for reload failed")
			}
		}
		vc = nil
		if startErr := startCore(); startErr != nil {
			log.WithFields(log.Fields{
				"err":    startErr,
				"reason": reason,
			}).Error("Restart V2bX failed")
			return
		}
		log.WithField("reason", reason).Info("V2bX reloaded")
		runtime.GC()
	}

	if watch {
		err = c.Watch(config, xdns, sdns, func() {
			reload("config", true)
		})
		if err != nil {
			log.WithField("err", err).Error("start watch failed")
			return
		}
	}
	startGeoUpdater(runtimeContext, geoManager, geoState, func() {
		reload("GeoFile", false)
	})
	// clear memory
	runtime.GC()
	// wait exit signal
	{
		osSignals := make(chan os.Signal, 1)
		signal.Notify(osSignals, syscall.SIGINT, syscall.SIGTERM)
		<-osSignals
	}
}
