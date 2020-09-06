package kubelan

import (
	"errors"
	"fmt"
	"net"
	"reflect"
	"regexp"

	"github.com/mitchellh/mapstructure"
	log "github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var objectMetaRegex = regexp.MustCompile(`^(\S+)/(\S+)$`)

// stringToLogLevelHookFunc returns a mapstructure.DecodeHookFunc which parses a logrus Level from a string
func stringToLogLevelHookFunc() mapstructure.DecodeHookFunc {
	return func(f reflect.Type, t reflect.Type, data interface{}) (interface{}, error) {
		if f.Kind() != reflect.String || t.Kind() != reflect.TypeOf(log.InfoLevel).Kind() {
			return data, nil
		}

		var level log.Level
		err := level.UnmarshalText([]byte(data.(string)))
		return level, err
	}
}

// stringOrDefaultToIPHookFunc returns a mapstructure.DecodeHookFunc which
// parses an IP (or gets the one on the same interface as the default gateway)
func stringOrDefaultToIPHookFunc() mapstructure.DecodeHookFunc {
	return func(f reflect.Type, t reflect.Type, data interface{}) (interface{}, error) {
		if f.Kind() != reflect.String || t != reflect.TypeOf(net.IP{}) {
			return data, nil
		}

		ip := data.(string)
		if ip != "" {
			parsed := net.ParseIP(ip)
			if parsed == nil {
				return nil, errors.New("invalid IP address")
			}

			return parsed, nil
		}

		routes, err := netlink.RouteList(nil, unix.AF_INET)
		if err != nil {
			return nil, fmt.Errorf("failed to list routes: %w", err)
		}

		var found net.IP
		for _, route := range routes {
			if route.Gw != nil {
				found = route.Src
				break
			}
		}
		if found == nil {
			return nil, errors.New("No IP address provided and failed to guess default")
		}

		return found, nil
	}
}

// stringToObjectMetaHookFunc returns a mapstructure.DecodeHookFunc which parses
// a string of form `<namespace>/<name>` into a Kubernetes ObjectMeta
func stringToObjectMetaHookFunc() mapstructure.DecodeHookFunc {
	return func(f reflect.Type, t reflect.Type, data interface{}) (interface{}, error) {
		if f.Kind() != reflect.String || t != reflect.TypeOf(v1.ObjectMeta{}) {
			return data, nil
		}

		value := data.(string)
		match := objectMetaRegex.FindStringSubmatch(value)
		if len(match) == 0 {
			return v1.ObjectMeta{Name: value}, nil
		}

		return v1.ObjectMeta{
			Namespace: match[1],
			Name:      match[2],
		}, nil
	}
}

// ConfigDecoderOptions enables necessary mapstructure decode hook functions
func ConfigDecoderOptions(config *mapstructure.DecoderConfig) {
	config.ErrorUnused = true
	config.DecodeHook = mapstructure.ComposeDecodeHookFunc(
		stringOrDefaultToIPHookFunc(),
		config.DecodeHook,
		stringToLogLevelHookFunc(),
		stringToObjectMetaHookFunc(),
	)
}

// Config defines the kubelan Manager's config
type Config struct {
	LogLevel log.Level `mapstructure:"log_level"`

	IP        net.IP
	Namespace string
	Services  []v1.ObjectMeta

	VXLAN struct {
		Interface string
		MTU       int
		VNI       uint32
		Port      uint16
	}

	Hooks struct {
		Up     []string
		Change []string
	}
}
