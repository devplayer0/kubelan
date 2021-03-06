package main

import (
	"os"
	"os/signal"
	"strings"

	"github.com/devplayer0/kubelan/pkg/kubelan"
	"github.com/fsnotify/fsnotify"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"golang.org/x/sys/unix"
	"k8s.io/client-go/tools/clientcmd"
)

var manager *kubelan.Manager

func init() {
	// Config defaults
	viper.SetDefault("log_level", log.InfoLevel)
	viper.SetDefault("http_address", ":8181")
	viper.SetDefault("ip", "")
	viper.SetDefault("namespace", "")
	viper.SetDefault("services", []string{})
	viper.SetDefault("vxlan.interface", "kubelan")
	viper.SetDefault("vxlan.mtu", -1)
	viper.SetDefault("vxlan.vni", 6969)
	viper.SetDefault("vxlan.port", 4789)
	viper.SetDefault("hooks.up", []string{})
	viper.SetDefault("hooks.change", []string{})

	// Config file loading
	viper.SetConfigType("yaml")
	viper.SetConfigName("kubelan")
	viper.AddConfigPath("/run/config")
	viper.AddConfigPath(".")

	// Config from environment
	viper.SetEnvPrefix("kl")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	// Config from flags
	pflag.StringP("log_level", "l", "info", "log level")
	pflag.Parse()
	if err := viper.BindPFlags(pflag.CommandLine); err != nil {
		log.WithError(err).Fatal("Failed to bind pflags to config")
	}

	if err := viper.ReadInConfig(); err != nil {
		log.WithError(err).Debug("Failed to read config")
	}
}

func stop() {
	if err := manager.Stop(); err != nil {
		log.WithError(err).Fatal("Failed to stop kubelan manager")
	}
}

func reloadConfig() {
	if manager != nil {
		stop()
		manager = nil
	}

	var config kubelan.Config
	if err := viper.Unmarshal(&config, kubelan.ConfigDecoderOptions); err != nil {
		log.WithField("err", err).Fatal("Failed to parse configuration")
	}

	log.SetLevel(config.LogLevel)
	log.WithField("config", config).Debug("Got config")

	k8sConf, err := clientcmd.BuildConfigFromFlags("", os.Getenv(clientcmd.RecommendedConfigPathEnvVar))
	if err != nil {
		log.WithError(err).Fatal("Failed to load Kubernetes config")
	}

	manager, err = kubelan.NewManager(k8sConf, config)
	if err != nil {
		log.WithError(err).Fatal("Failed to create kubelan manager")
	}

	if err := manager.Start(); err != nil {
		log.WithError(err).Fatal("Failed to start kubelan manager")
	}
}

func main() {
	sigs := make(chan os.Signal)
	signal.Notify(sigs, unix.SIGINT, unix.SIGTERM)

	viper.OnConfigChange(func(e fsnotify.Event) {
		log.WithField("file", e.Name).Info("Config changed, reloading")
		reloadConfig()
	})
	viper.WatchConfig()
	reloadConfig()

	<-sigs
	stop()
}
