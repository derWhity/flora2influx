package main

import (
	"fmt"
	"os"
	"time"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
)

// Configuration is the main application configuration file
type Configuration struct {
	Influx     InfluxConfig     `yaml:"influx"`
	Collection CollectionConfig `yaml:"collection"`
	Devices    DeviceConfigMap  `yaml:"devices"`
}

// Validate checks the values in the configuration for errornous values
func (c *Configuration) Validate() error {
	if c == nil {
		return fmt.Errorf("No configuration provided")
	}
	if c.Collection.DiscoveryInterval < time.Minute {
		return fmt.Errorf("Discovery interval of %s is too low. Please use an interval greater or equal one minute", c.Collection.DiscoveryInterval)
	}
	if c.Collection.Interval < time.Minute {
		return fmt.Errorf("Collection interval of %s is too low. Please use an interval greater or equal one minute", c.Collection.Interval)
	}
	if c.Collection.Interval > c.Collection.DiscoveryInterval {
		return fmt.Errorf("Collection interval is greater than the rediscovery interval (%s > %s). Please use a smaller value for the collection interval",
			c.Collection.Interval,
			c.Collection.DiscoveryInterval,
		)
	}
	if c.Collection.DiscoveryCooldown < time.Second {
		return fmt.Errorf("Discovery cooldown of %s is too low. Please use an interval greater or equal one second", c.Collection.DiscoveryCooldown)
	}
	if c.Collection.DiscoveryTimeout < time.Second*5 {
		return fmt.Errorf("Discovery timeout of %s is too low. Please use an interval greater or equal five seconds", c.Collection.Interval)
	}
	return nil
}

// InfluxConfig configures the connection to the InfluxDB
type InfluxConfig struct {
	// Address the InfluxDB instance is listening at
	Addr string `yaml:"addr"`
	// Optional user name for authentication
	Username string `yaml:"username"`
	// Optional password for authentication
	Password string `yaml:"password"`
	// The database to use (has to exist!)
	Database string `yaml:"database"`
	// The name of the measurement to write into
	MeasurementName string `yaml:"measurement"`
}

// CollectionConfig configures the data collection options of this application
type CollectionConfig struct {
	// The interval between two automatic device discoveries
	DiscoveryInterval time.Duration `yaml:"discoveryInterval"`
	// The interval after the scan for devices will stop
	DiscoveryTimeout time.Duration `yaml:"discoveryTimeout"`
	// The time the app waits when a discovery attempt has failed before retrying
	DiscoveryCooldown time.Duration `yaml:"discoveryCooldown"`
	// Interval at which the readings are fetched from the discovered device(s)
	Interval time.Duration `yaml:"interval"`
}

// DeviceConfigMap maps the MAC address of a device to the device's configuration
type DeviceConfigMap map[string]DeviceConfig

// DeviceConfig holds the collection config for a single flora device identified by its MAC
type DeviceConfig struct {
	// An alias name for the device. This will be written to the tags of each entry
	Alias string `yaml:"alias"`
	// If set to true, this device will be ignored during collection
	Ignore bool `yaml:"ignore"`
}

func getDefaultConfig() *Configuration {
	return &Configuration{
		Influx: InfluxConfig{
			Addr:            "http://localhost:8086",
			Database:        "flora",
			MeasurementName: "PlantSensorReadings",
		},
		Collection: CollectionConfig{
			DiscoveryInterval: time.Hour,
			DiscoveryCooldown: time.Second * 30,
			DiscoveryTimeout:  time.Second * 10,
			Interval:          time.Minute,
		},
	}
}

func loadConfigFile(filename string) (*Configuration, error) {
	conf := getDefaultConfig()
	f, err := os.Open(filename)
	if err != nil {
		return nil, errors.Wrap(err, "Cannot open configuration file")
	}
	defer f.Close()
	if err := yaml.NewDecoder(f).Decode(conf); err != nil {
		return nil, errors.Wrap(err, "Failed to load configuration file")
	}
	if err := conf.Validate(); err != nil {
		return nil, errors.Wrap(err, "Errors found in the configuration")
	}
	return conf, nil
}
