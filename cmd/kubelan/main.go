package main

import (
	"os"
	"os/signal"

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
	viper.SetDefault("ip", "")
	viper.SetDefault("services", []string{})
	viper.SetDefault("interface", "kubelan")
	viper.SetDefault("vid", 6969)

	// Config file loading
	viper.SetConfigType("yaml")
	viper.SetConfigName("kubelan")
	viper.AddConfigPath("/run/config")
	viper.AddConfigPath(".")

	// Config from environment
	viper.SetEnvPrefix("kl")
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

func reloadConfig() {
	if manager != nil {
		manager.Stop()
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

	manager.Start()
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
	manager.Stop()
}
