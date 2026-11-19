package config

import (
	"bytes"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// configV1 is the original PHP-centric schema (version: 1). It is only read;
// SLD upgrades it to the version 2 shape in memory or via `config migrate`.
type configV1 struct {
	Version int    `yaml:"version"`
	Name    string `yaml:"name,omitempty"`
	Runtime struct {
		PHP     string `yaml:"php"`
		WebRoot string `yaml:"web_root,omitempty"`
	} `yaml:"runtime"`
	Domain   DomainConfig   `yaml:"domain,omitempty"`
	Services ServicesConfig `yaml:"services,omitempty"`
	Node     struct {
		Version   string `yaml:"version,omitempty"`
		DevServer struct {
			Enabled    bool   `yaml:"enabled,omitempty"`
			Command    string `yaml:"command,omitempty"`
			Port       int    `yaml:"port,omitempty"`
			ExposePort int    `yaml:"expose_port,omitempty"`
		} `yaml:"dev_server,omitempty"`
	} `yaml:"node,omitempty"`
	Xdebug struct {
		Enabled bool `yaml:"enabled,omitempty"`
	} `yaml:"xdebug,omitempty"`
	Environment map[string]string `yaml:"environment,omitempty"`
}

// Names of the apps a version 1 config becomes.
const (
	V1WebApp  = "web"
	V1ViteApp = "vite"
)

// upgradeV1 converts a version 1 config to version 2 without applying defaults,
// so the result can be written back as a minimal file. Values that version 1
// filled in implicitly (dev server port/command) are made explicit, because
// version 2 node apps have different defaults.
func upgradeV1(old *configV1) (*Config, error) {
	if old.Runtime.PHP == "" {
		return nil, fmt.Errorf("runtime.php is required in version 1 configs (supported: %s); "+
			"to use another runtime, switch to 'version: 2' with an 'apps:' section", strings.Join(SupportedPHPVersions, ", "))
	}

	cfg := &Config{
		Version:     CurrentVersion,
		Name:        old.Name,
		Domain:      old.Domain,
		Services:    old.Services,
		Environment: old.Environment,
		Apps: Apps{{
			Name:    V1WebApp,
			Runtime: RuntimePHP,
			Version: old.Runtime.PHP,
			WebRoot: old.Runtime.WebRoot,
			Xdebug:  old.Xdebug.Enabled,
		}},
		SourceVersion: 1,
	}

	dev := old.Node.DevServer
	if dev.Enabled {
		vite := App{
			Name:       V1ViteApp,
			Runtime:    RuntimeNode,
			Version:    old.Node.Version,
			Command:    dev.Command,
			Port:       dev.Port,
			ExposePort: dev.ExposePort,
		}
		if vite.Command == "" {
			vite.Command = "npm run dev"
		}
		if vite.Port == 0 {
			vite.Port = 5173
		}
		if vite.ExposePort == 0 {
			vite.ExposePort = 5173
		}
		cfg.Apps = append(cfg.Apps, vite)
	} else if old.Node.Version != "" {
		// Version 1 used node.version for `sld npm` without a dev server.
		cfg.Tools.Node = old.Node.Version
	}

	return cfg, nil
}

// MigrateV1 converts a version 1 config file to version 2 YAML. It returns
// ErrAlreadyCurrent for files that are already version 2.
func MigrateV1(data []byte) ([]byte, error) {
	version, err := peekVersion(data)
	if err != nil {
		return nil, err
	}
	if version == CurrentVersion {
		return nil, ErrAlreadyCurrent
	}
	if version != 1 {
		return nil, unsupportedVersion(version)
	}

	var old configV1
	if err := yaml.Unmarshal(data, &old); err != nil {
		return nil, fmt.Errorf("malformed YAML configuration: %w", err)
	}
	cfg, err := upgradeV1(&old)
	if err != nil {
		return nil, err
	}
	return Marshal(cfg)
}

// Marshal encodes a config as YAML with SLD's standard header.
func Marshal(cfg *Config) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString("# SLD project configuration — https://github.com/supreme-majesty/supreme-local-dev\n")
	buf.WriteString("# Each app under 'apps:' is served on its own domain; the first app owns the root domain.\n\n")
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(cfg); err != nil {
		return nil, fmt.Errorf("failed to encode configuration: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
