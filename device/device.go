package device

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/paypal/gatt"
	"github.com/paypal/gatt/examples/option"
	"github.com/sirupsen/logrus"
	"github.com/pkg/errors"
)

const (
	fldDevice = "device"
	fldAlias  = "alias"
	// VHandle of the realtime data switching characteristic. You need to write 0xA01F to this in order to start
	// the real-time data mode. Otherwise, the sensor readings will return a static value
	vHandleRealtimeData = 0x33
	// VHandle of the firmware and battery characteristic.
	// Firmware version and battery charge (in %) can be read from here
	vHandleFirmwareAndBattery = 0x38
	// VHandle of the sensor readings characteristic.
	// Reading from this provides current temperature, light intensity, moisture and fertility readings
	vHandleSensorReadings = 0x35

	//-- Influx value names

	keyBatteryLevel = "battery"
	keyTemperature  = "temperature"
	keyMoisture     = "moisture"
	keyConductivity = "conductivity"
	keyLight        = "light"
)

var (
	// Device names the MiFlora device is known to identify with
	floraDeviceNames = map[string]int{
		"Flower care": 1,
		"Flower mate": 1,
	}
	// The UUID of the service that holds all three characteristics we need to retrieve data
	floraServiceUUID = gatt.MustParseUUID("0000120400001000800000805f9b34fb")
)

// Discover runs a discovery on the local network for routers providing the service
// "urn:schemas-upnp-org:service:WANCommonInterfaceConfig:1" via UPnP and returns a list
// of all of those devices found
//
// The scanning will happen for the time-range given in `timeout` before it will stop
// automatically
func Discover(timeout time.Duration, confMap ConfigMap, logger *logrus.Entry) ([]*Device, error) {
	logger.Info("Discovering Bluetooth devices in the vincinity...")
	out := []*Device{}
	btDev, err := gatt.NewDevice(option.DefaultClientOptions...)
	if err != nil {
		logger.WithError(err).Error("Failed to create a new GATT device")
		return errors.Wrap(err, "Failed to create a new GATT device")
	}
	btDev.Handle(gatt.PeripheralDiscovered(func(p gatt.Peripheral, a *gatt.Advertisement, rssi int) {
		if _, ok := floraDeviceNames[p.Name()]; ok {
			dev := &Device{
				Logger:     logger.WithField(fldDevice, p.ID()),
				peripheral: p,
			}
			if conf, ok := confMap[p.ID()]; ok {
				if conf.Ignore {
					dev.Logger.Infof("Device will be ignored")
					return
				}
				if conf.Alias != "" {
					dev.Logger = dev.Logger.WithField(fldAlias, conf.Alias)
					dev.Alias = conf.Alias
				}
			}
			dev.Logger.Info("Flora device detected")
			out = append(out, dev)
		}
	}))
	btDev.Init(func(d gatt.Device, s gatt.State) {
		logger.Infof("Device state changed to '%s'", s)
		switch s {
		case gatt.StatePoweredOn:
			logger.Info("Device is up. Scan is starting...")
			d.Scan([]gatt.UUID{}, false)
			return
		default:
			d.StopScanning()
		}
	})
	time.Sleep(timeout)
	logger.Infof("Stopping the scan after %s", timeout)
	btDev.StopScanning()
	return out, nil
}

// Readings represents one set of readings received from the Flora device
type Readings struct {
	// Version string of the firmware
	FirmwareVersion string
	// Battery level in percent
	BatteryLevel uint8
	// Temperature in °C
	Temperature float64
	// Moisture in percent
	Moisture byte
	// Light in lumens
	Light uint16
	// Conductivity in µS/cm
	Conductivity uint16
}

func (r *Readings) String() string {
	return fmt.Sprintf(
		"[ 🔋 %d | 🌡  %.1f°C | 💧 %d%% | 💡 %d lm | ⚡️ %d µS/cm | v%s ]",
		r.BatteryLevel,
		r.Temperature,
		r.Moisture,
		r.Light,
		r.Conductivity,
		r.FirmwareVersion,
	)
}

// ToInfluxValues returns the reading values as influx field values
func (r *Readings) ToInfluxValues() map[string]interface{} {
	return map[string]interface{}{
		keyBatteryLevel: r.BatteryLevel,
		keyTemperature:  r.Temperature,
		keyMoisture:     r.Moisture,
		keyConductivity: r.Conductivity,
		keyLight:        r.Light,
	}
}

// Device represents a router device found during discovery
type Device struct {
	// The peripheral found
	peripheral gatt.Peripheral
	// Logger entry that is preconfigured with fields identifying the router
	Logger *logrus.Entry
	// The alias if configured
	Alias string
}

// GetName returns the device's alias or MAC address - depending on what is available
func (dev *Device) GetName() string {
	if dev.Alias != "" {
		return dev.Alias
	}
	return dev.GetID()
}

// GetID returns the device's MAC address (ID)
func (dev *Device) GetID() string {
	return dev.peripheral.ID()
}

// FetchReadings tries to fetch the current readings from the device
func (dev *Device) FetchReadings() (*Readings, error) {
	var errOut error
	var out *Readings
	dev.Logger.Info("Fetching readings from device")
	done := make(chan bool)
	dev.peripheral.Device().Handle(
		gatt.PeripheralConnected(func(p gatt.Peripheral, err error) {
			defer p.Device().CancelConnection(p)
			dev.Logger.Debug("Connection to device established")
			services, err := p.DiscoverServices(nil)
			if err != nil {
				dev.Logger.WithError(err).Error("Service disvovery failed on device")
				return
			}
			var cFirmware *gatt.Characteristic
			var cReadings *gatt.Characteristic
			var cRealtimeData *gatt.Characteristic
			for _, service := range services {
				if service.UUID().Equal(floraServiceUUID) {
					dev.Logger.Debugf("Found sensor data service on device (%s)", floraServiceUUID)
					characteristics, err := p.DiscoverCharacteristics(nil, service)
					if err != nil {
						dev.Logger.WithError(err).Error("Characteristics disvovery failed on device")
						errOut = err
						return
					}
					for _, characteristic := range characteristics {
						switch characteristic.VHandle() {
						case vHandleRealtimeData:
							cRealtimeData = characteristic
							dev.Logger.Debugf("Found realtime data switch characteristic (0x%x)", characteristic.VHandle())
						case vHandleSensorReadings:
							cReadings = characteristic
							dev.Logger.Debugf("Found sensor reading characteristic (0x%x)", characteristic.VHandle())
						case vHandleFirmwareAndBattery:
							cFirmware = characteristic
							dev.Logger.Debugf("Found firmware and battery data characteristic (0x%x)", characteristic.VHandle())
						}
					}
				}
			}
			if cFirmware == nil {
				dev.Logger.Error("No firmware characteristic found. Aborting query.")
				errOut = fmt.Errorf("No firmware and battery characteristic found on device")
				return
			}
			// Get the firmware version in order to determine if we need to enable real-time data beforehand
			rd := &Readings{}
			data, err := p.ReadCharacteristic(cFirmware)
			if err != nil {
				dev.Logger.WithError(err).Error("Failed reading firmware data")
				errOut = err
				return
			}
			decodeFirmwareData(data, rd)
			dev.Logger.Debugf("Firmware version: %s - Battery at %d%%", rd.FirmwareVersion, rd.BatteryLevel)

			// For firmware later than 2.6.6 we need to enable realtime data read in order to get any sensor data
			if rd.FirmwareVersion > "2.6.6" {
				if cRealtimeData == nil {
					dev.Logger.Error("No realtime data switch characteristic dicovered. Sensor will not return proper data - aborting.")
					errOut = fmt.Errorf("Realtime data switch characteristic not found")
					return
				}
				if err := p.WriteCharacteristic(cRealtimeData, []byte{0xa0, 0xaf}, false); err != nil {
					dev.Logger.WithError(err).Error("Failed to enable realtime data reading")
					errOut = err
					return
				}
				dev.Logger.Debug("Realtime data reading enabled on device")
			}

			if cReadings == nil {
				dev.Logger.Error("No readings characteristic discovered. Unable to read sensor data.")
				errOut = fmt.Errorf("No readings characteristic discovered on device")
				return
			}

			data, err = p.ReadLongCharacteristic(cReadings)
			if err != nil {
				dev.Logger.WithError(err).Error("Failed reading sensor data")
				errOut = err
				return
			}
			decodeSensorData(data, rd)
			out = rd
		}),
		gatt.PeripheralDisconnected(func(p gatt.Peripheral, err error) {
			dev.Logger.Debug("Disconnected from device")
			close(done)
		}),
	)
	dev.peripheral.Device().Connect(dev.peripheral)
	<-done
	return out, errOut
}

func decodeFirmwareData(data []byte, rd *Readings) {
	buf := bytes.NewBuffer(data)
	var batt uint8
	binary.Read(buf, binary.LittleEndian, &batt)
	rd.BatteryLevel = batt
	buf.Next(1)
	// The rest is the version string
	rd.FirmwareVersion = buf.String()
}

func decodeSensorData(data []byte, rd *Readings) {
	p := bytes.NewBuffer(data)
	var t int16
	var m uint8
	var l, c uint16

	// Data format: TT TT ?? LL LL ?? ?? MM CC CC
	//             |Temp |  |Light|     |⬇︎| Conductivity
	//                                Moisture
	binary.Read(p, binary.LittleEndian, &t)
	rd.Temperature = float64(t) / 10
	p.Next(1)
	binary.Read(p, binary.LittleEndian, &l)
	rd.Light = l
	p.Next(2)
	binary.Read(p, binary.LittleEndian, &m)
	rd.Moisture = m
	binary.Read(p, binary.LittleEndian, &c)
	rd.Conductivity = c
}
