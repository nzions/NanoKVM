package config

import (
	"bytes"
	"errors"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

// etcDir is the directory holding every NanoKVM state file. initialize()
// overrides it once from etcDirEnv, so tests can run off-device.
var etcDir = "/etc/kvm"

const etcDirEnv = "NANOKVM_ETC_DIR"

var (
	instance Config
	once     sync.Once
)

// etcPath returns the path of name inside etcDir.
func etcPath(name string) string {
	return filepath.Join(etcDir, name)
}

func GetInstance() *Config {
	once.Do(initialize)

	return &instance
}

func initialize() {
	if dir := os.Getenv(etcDirEnv); dir != "" {
		etcDir = dir
	}

	if err := readByFile(); err != nil {
		if errors.As(err, &viper.ConfigFileNotFoundError{}) {
			create()
		}

		if err = readByDefault(); err != nil {
			log.Fatalf("Failed to read default configuration!")
		}

		log.Println("using default configuration")
	}

	if err := validate(); err != nil {
		log.Fatalf("Failed to validate configuration!")
	}

	if err := viper.Unmarshal(&instance); err != nil {
		log.Fatalf("Failed to parse configuration: %s", err)
	}

	if err := checkDefaultValue(); err != nil {
		log.Fatalf("Failed to finalize configuration: %s", err)
	}

	if instance.Authentication == "disable" {
		log.Println("NOTICE: Authentication is disabled! Please ensure your service is secure!")
	}

	log.Println("config loaded successfully")
}

func readByFile() error {
	viper.SetConfigName("server")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(etcDir)

	return viper.ReadInConfig()
}

func readByDefault() error {
	data, err := yaml.Marshal(defaultConfig())
	if err != nil {
		log.Printf("failed to marshal default config: %s", err)
		return err
	}

	return viper.ReadConfig(bytes.NewBuffer(data))
}

// Create configuration file.
func create() {
	var (
		file *os.File
		data []byte
		err  error
	)

	_ = os.MkdirAll(etcDir, 0o644)

	file, err = os.OpenFile(ConfigurationFile(), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		log.Printf("open config failed: %s", err)
		return
	}
	// the file may already exist with a looser mode
	if err = file.Chmod(0o600); err != nil {
		log.Printf("chmod config failed: %s", err)
	}
	defer func() {
		_ = file.Close()
	}()

	if data, err = yaml.Marshal(defaultConfig()); err != nil {
		log.Printf("failed to marshal default config: %s", err)
		return
	}

	if _, err = file.Write(data); err != nil {
		log.Printf("failed to save config: %s", err)
		return
	}

	if err = file.Sync(); err != nil {
		log.Printf("failed to sync config: %s", err)
		return
	}

	log.Printf("create file %s with default configuration", ConfigurationFile())
}

// Validate the configuration. This is to ensure compatibility with earlier versions.
func validate() error {
	if viper.GetInt("port.http") > 0 && viper.GetInt("port.https") > 0 {
		return nil
	}

	_ = os.Remove(ConfigurationFile())
	log.Println("delete empty configuration file")

	create()

	return readByDefault()
}
