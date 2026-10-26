package config

type Config struct {
	Proto          string   `yaml:"proto"`
	Host           string   `yaml:"host"`
	Port           Port     `yaml:"port"`
	Cert           Cert     `yaml:"cert"`
	Logger         Logger   `yaml:"logger"`
	Authentication string   `yaml:"authentication"`
	JWT            JWT      `yaml:"jwt"`
	Stun           string   `yaml:"stun"`
	Turn           Turn     `yaml:"turn"`
	Security       Security `yaml:"security"`
	// Firewall is read by kvmapp/system/nanokvm-firewall, not by the server:
	// "open" (default) or "drop". Declared here so Write() keeps the key.
	Firewall string `yaml:"firewall"`

	Hardware Hardware `yaml:"-"`
}

type Logger struct {
	Level string `yaml:"level"`
	File  string `yaml:"file"`
}

type Port struct {
	Http  int `yaml:"http"`
	Https int `yaml:"https"`
}

type Cert struct {
	Crt string `yaml:"crt"`
	Key string `yaml:"key"`
}

type JWT struct {
	SecretKey            string `yaml:"secretKey"`
	RefreshTokenDuration uint64 `yaml:"refreshTokenDuration"`
	RevokeTokensOnLogout bool   `yaml:"revokeTokensOnLogout"`
}

type Turn struct {
	TurnAddr string `yaml:"turnAddr"`
	TurnUser string `yaml:"turnUser"`
	TurnCred string `yaml:"turnCred"`
}

type Security struct {
	LoginLockoutDuration int `yaml:"loginLockoutDuration"`
	LoginMaxFailures     int `yaml:"loginMaxFailures"`
	// TrustedProxies are the IPs/CIDRs of reverse proxies whose X-Forwarded-Proto
	// is believed. Empty (the default) trusts nobody.
	TrustedProxies []string `yaml:"trustedProxies"`
	// CORSOrigins are the origins allowed cross-origin access. Empty (the
	// default) means same-origin only.
	CORSOrigins []string `yaml:"corsOrigins"`
}

// Hardware names the board's GPIO lines twice: Line* are the device-tree line
// names (the GPIO chardev, the only path on the mainline kernel), GPIO* the
// vendor kernel's sysfs value files, used only when no line carries the name.
type Hardware struct {
	Version      HWVersion `yaml:"-"`
	LineReset    string    `yaml:"-"`
	LinePower    string    `yaml:"-"`
	LinePowerLED string    `yaml:"-"`
	LineHDDLed   string    `yaml:"-"`
	GPIOReset    string    `yaml:"-"`
	GPIOPower    string    `yaml:"-"`
	GPIOPowerLED string    `yaml:"-"`
	GPIOHDDLed   string    `yaml:"-"`
}
