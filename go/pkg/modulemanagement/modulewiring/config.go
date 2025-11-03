package modulewiring

import (
	"fmt"
	"os"
	"time"

	"github.com/core-tools/hsu-core/pkg/modulemanagement/moduleproto"
	"github.com/core-tools/hsu-core/pkg/modulemanagement/moduletypes"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Runtime RuntimeConfig  `yaml:"runtime"`
	Modules []ModuleConfig `yaml:"modules"`
}

type RuntimeConfig struct {
	Servers         []ServerConfig        `yaml:"servers"`
	ServiceRegistry ServiceRegistryConfig `yaml:"service_registry"`
}

type ServerConfig struct {
	ID       moduleproto.ServerID `yaml:"id"`
	Protocol moduletypes.Protocol `yaml:"protocol"`
	Options  map[string]any       `yaml:"options"`
	Enabled  bool                 `yaml:"enabled"`
}

type ServiceRegistryConfig struct {
	URL     string        `yaml:"url"`
	Timeout time.Duration `yaml:"timeout"`
}

type ModuleConfig struct {
	ID      moduletypes.ModuleID   `yaml:"id"`
	Servers []moduleproto.ServerID `yaml:"servers"`
	Enabled bool                   `yaml:"enabled"`
}

func LoadConfig(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	return &cfg, nil
}
