package config

import "fmt"

// defaultLoginLockoutDuration is the lockout in seconds. 0 (or unset) means
// this default; a negative value disables the lockout.
const defaultLoginLockoutDuration = 300

var defaultConfig = &Config{
	Proto: "https",
	Host:  "",
	Port: Port{
		Http:  80,
		Https: 443,
	},
	Cert: Cert{
		Crt: "/etc/kvm/server.crt",
		Key: "/etc/kvm/server.key",
	},
	Logger: Logger{
		Level: "info",
		File:  "stdout",
	},
	JWT: JWT{
		SecretKey:            "",
		RefreshTokenDuration: 2678400,
		RevokeTokensOnLogout: true,
	},
	Stun: "stun.l.google.com:19302",
	Turn: Turn{
		TurnAddr: "",
		TurnUser: "",
		TurnCred: "",
	},
	Authentication: "enable",
	Security: Security{
		LoginLockoutDuration: defaultLoginLockoutDuration,
		LoginMaxFailures:     5,
	},
}

func checkDefaultValue() error {
	if instance.JWT.SecretKey == "" {
		secret, err := loadOrCreateJWTSecret(jwtSecretFile)
		if err != nil {
			return fmt.Errorf("jwt secret: %w", err)
		}
		instance.JWT.SecretKey = secret
		instance.JWT.RevokeTokensOnLogout = true
	}

	if instance.JWT.RefreshTokenDuration == 0 {
		instance.JWT.RefreshTokenDuration = 2678400
	}

	if instance.Stun == "" {
		instance.Stun = "stun.l.google.com:19302"
	}

	if instance.Authentication == "" {
		instance.Authentication = "enable"
	}

	// Configs written by older releases carry an explicit 0 (then "off"); 0
	// now means the default so those devices get the lockout too.
	if instance.Security.LoginLockoutDuration == 0 {
		instance.Security.LoginLockoutDuration = defaultLoginLockoutDuration
	}

	if instance.Security.LoginMaxFailures <= 0 {
		instance.Security.LoginMaxFailures = 5
	}

	instance.Hardware = getHardware()

	return nil
}
