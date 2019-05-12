package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/derWhity/flora2influx/device"
	client "github.com/influxdata/influxdb1-client/v2"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
)

const (
	appName = "flora2influx"
	version = "0.0.1"

	// Tag names
	tagMAC             = "mac"
	tagAlias           = "alias"
	tagFirmwareVersion = "version"
)

func discoverAndRun(errChan chan error, config *Configuration, influxClient client.Client, logger *logrus.Entry) {
	devices, err := device.Discover(config.Collection.DiscoveryTimeout, logger)
	if err != nil {
		logger.WithError(err).Error("Device discovery failed")
		errChan <- err
		return
	}
	devStr := "devices"
	if len(devices) == 1 {
		devStr = "device"
	}
	logger.Infof("Scan finished. %d %s found", len(devices), devStr)
	// Forces re-discovery after a given period of time
	reloadTimer := time.NewTimer(config.Collection.DiscoveryInterval)
	tickTimer := time.NewTicker(config.Collection.Interval)
	for {
		/*
			batch, err := client.NewBatchPoints(client.BatchPointsConfig{
				Database:  config.Influx.Database,
				Precision: "s",
			})
			if err != nil {
				logger.WithError(err).Error("Failed to create point batch configuration")
				errChan <- err
				return
			}
		*/
		for _, device := range devices {
			readings, err := device.FetchReadings()
			if err != nil {
				device.Logger.WithError(err).Error("Failed to fetch readings from device")
				continue
			}
			device.Logger.Infof("Received readings: %s", readings)
			/*
				tags := map[string]string{
					tagManufacturer: device.RootDevice.Device.Manufacturer,
					tagModel:        device.RootDevice.Device.ModelName,
					tagHost:         device.RootDevice.URLBase.Hostname(),
					tagUDN:          device.RootDevice.Device.UDN,
				}
				pt, err := client.NewPoint(config.Influx.MeasurementName, tags, readings.ToInfluxValues(), time.Now())
				if err != nil {
					device.Logger.WithError(err).Error("Failed to create data point for measurements")
				}
				batch.AddPoint(pt)
			*/
		}
		/*
			logger.Info("Exporting batch data to InfluxDB")
			// Send the collected info to Influx
			if err = influxClient.Write(batch); err != nil {
				logger.WithError(err).Error("Failed to upload data to InfluxDB")
			} else {
				logger.Info("Batch successfully uploaded")
			}
		*/
		// And now we'll wait
		select {
		case <-reloadTimer.C:
			tickTimer.Stop()
			close(errChan)
			return
		case <-tickTimer.C:
			// Nothing to do here
		}
	}
}

func main() {
	configFileName := flag.String("c", fmt.Sprintf("/etc/%[1]s/%[1]s.conf", appName), "Configuration file to load")
	dumpDefaultConfiguration := flag.Bool("dump", false, "Dump the default configuration to stdout. Useful for creating a config file")
	flag.Parse()

	if *dumpDefaultConfiguration {
		data, _ := yaml.Marshal(getDefaultConfig())
		fmt.Println(string(data))
		return
	}

	logrus.SetLevel(logrus.DebugLevel)
	logger := logrus.WithField("ver", version)
	logger.Infof("%s v%s starting up", appName, version)

	config, err := loadConfigFile(*configFileName)
	if err != nil {
		logger.Fatalf("Failed to load configuration: %+v", err)
	}

	// Influx client
	iConfig := client.HTTPConfig{
		Addr: config.Influx.Addr,
	}
	if config.Influx.Username != "" {
		iConfig.Username = config.Influx.Username
		iConfig.Password = config.Influx.Password
	}
	influxClient, err := client.NewHTTPClient(iConfig)
	if err != nil {
		logger.Fatalf("Failed to create InfluxDB client: %+v", err)
	}
	shutdown := make(chan os.Signal)
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM)
	for {
		errChan := make(chan error)
		go discoverAndRun(errChan, config, influxClient, logger)
		select {
		case err, ok := <-errChan:
			if ok {
				// An error occured - slow down the device discovery a bit
				logger.WithError(err).Error("Re-scheduling discovery in 10 seconds")
				time.Sleep(config.Collection.DiscoveryCooldown)
			}
			logger.Info("Restarting discovery")
		case sig := <-shutdown:
			logger.Infof("Got signal to stop (%s). Shutting down", sig)
			influxClient.Close()
			return
		}
	}
}
