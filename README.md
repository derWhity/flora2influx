# flora2influx

A simple tool to read the current sensor readings from a Mi Flora plant sensor and write it to an [InfluxDB](https://www.influxdata.com/) instance.

**Note**:<br/>
This project is using [Paypal's Bluetooth Low Energy package|https://github.com/paypal/gatt] which means that it will access the HCI device exclusively - no other program has access
to it while this tool is running.

## Status of this project

This tool is currently under development and is not ready for use. Once it reaches a first usable state, a pre-alpha release will be made.

## Installation

The built version of `flora2influx` is self-containing. Just download the binary release from the "Releases" section, extract to the directory of your choice and run it.

Because the tool administers network devices, it must either be run as root, or be granted appropriate capabilities:

```
sudo flora2influx
# OR
sudo setcap 'cap_net_raw,cap_net_admin=eip' flora2influx
flora2influx
```

### Building it yourself

The only prerequisite to build `flora2influx` is a recent installation of the `go` tools obtainable at https://golang.org/dl. Once this is on your system, clone this repository into a directory of your choice
```
> git clone https://github.com/derWhity/flora2influx.git
> cd flora2influx
```
and build it via
```
> go build
```
This will download all dependencies for the project and build a binary for your system.

## Usage

Once started, `flora2influx` will start searching for Flora sensors in the vincinity. This scan will be repeated in an default interval of one hour to add new devices that have been started since or remove those which have gone out of range. Once the discovery has finished, flora2influx will start querying all sensors it found for the current sensor measurements and sends those to the configured InfluxDB server. This repeats every minute by default.

Both timeout values can be changed in the configuration file which the tool tries to load from a default location (`/etc/flora2influx/flora2influx.conf`). In this file, you can also configure the location and credential of your InfluxDB instance.

The `flora2influx` command has two parameters:

* `-c` allows you to select a configuration file to use - e.g. `./flora2influx -c /home/pi/flora2influx.conf`
* `-dump` will dump flora2influx's default configuration to the standard output. You can use it to write a new configuration file:
```
> ./flora2influx -dump
influx:
  addr: http://localhost:8086
  username: ""
  password: ""
  database: flora
  measurement: PlantSensorReadings
collection:
  discoveryInterval: 1h0m0s
  discoveryTimeout: 10s
  discoveryCooldown: 30s
  interval: 1m0s
devices: {}
>
```