package device

// ConfigMap maps the MAC address of a device to the device's configuration
type ConfigMap map[string]Config

// Config holds the collection config for a single flora device identified by its MAC
type Config struct {
	// An alias name for the device. This will be written to the tags of each entry
	Alias string `yaml:"alias"`
	// If set to true, this device will be ignored during collection
	Ignore bool `yaml:"ignore"`
}
