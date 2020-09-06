package main

import (
	"flag"
	"os"
	"os/signal"

	"github.com/devplayer0/kubelan/pkg/kubelan"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
	"k8s.io/client-go/tools/clientcmd"
)

var logLevel = flag.String("loglevel", "info", "log level")

func init() {
	flag.Parse()

	level, err := log.ParseLevel(*logLevel)
	if err != nil {
		log.WithError(err).Fatal("Failed to parse log level")
	}
	log.SetLevel(level)
}

func main() {
	config, err := clientcmd.BuildConfigFromFlags("", os.Getenv(clientcmd.RecommendedConfigPathEnvVar))
	if err != nil {
		log.WithError(err).Fatal("Failed to load Kubernetes config")
	}

	manager, err := kubelan.NewManager(config)
	if err != nil {
		log.WithError(err).Fatal("Failed to create kubelan manager")
	}

	sigs := make(chan os.Signal)
	signal.Notify(sigs, unix.SIGINT, unix.SIGTERM)

	manager.Start()

	<-sigs
	manager.Stop()
}
