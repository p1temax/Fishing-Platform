package config

import (
	"crypto/sha256"
	"encoding/base32"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	defaultRuntimeConfigPath    = "./config.yaml"
	defaultGinMode              = "debug"
	defaultPort                 = 8000
	defaultDBPath               = "./data/fishing.db"
	defaultContainerSSLCertPath = "/app/certificates/cert.pem"
	defaultContainerSSLKeyPath  = "/app/certificates/key.pem"

	exampleEncryptionKey   = "replace-with-a-generated-key"
	exampleJWTSecret       = "replace-with-a-long-random-secret"
	exampleContainerSecret = "replace-with-another-long-random-secret"
	exampleAgentToken      = "replace-with-agent-registration-token"
)

type RuntimeFileConfig struct {
	Server struct {
		Port    int    `yaml:"port"`
		GinMode string `yaml:"gin_mode"`
	} `yaml:"server"`
	Database struct {
		Path string `yaml:"path"`
	} `yaml:"database"`
	Security struct {
		EncryptionKey            string `yaml:"encryption_key"`
		JWTSecret                string `yaml:"jwt_secret"`
		ContainerSecret          string `yaml:"container_secret"`
		AgentRegistrationToken   string `yaml:"agent_registration_token"`
		PlatformBasicAuthEnabled bool   `yaml:"platform_basic_auth_enabled"`
	} `yaml:"security"`
	Container struct {
		SSLCertPath string `yaml:"ssl_cert_path"`
		SSLKeyPath  string `yaml:"ssl_key_path"`
	} `yaml:"container"`
	// PublicBaseURL is the externally reachable origin used in mail tracking links.
	PublicBaseURL string `yaml:"public_base_url"`
	MailTracking  struct {
		Enabled            *bool    `yaml:"enabled"`
		Secret             string   `yaml:"secret"`
		AllowRedirectHosts []string `yaml:"allow_redirect_hosts"`
		// Optional opaque public paths (look like /api/xxxx). Empty = derive from secret.
		ClickPath string `yaml:"click_path"`
		OpenPath  string `yaml:"open_path"`
	} `yaml:"mail_tracking"`
}

type InsecureSecuritySetting struct {
	Path   string
	Label  string
	Reason string
}

var runtimeConfig RuntimeFileConfig

func BootstrapRuntimeConfig() (string, error) {
	cfg, err := loadRuntimeFileConfig(defaultRuntimeConfigPath)
	if err != nil {
		return "", err
	}

	applyRuntimeDefaults(&cfg)
	runtimeConfig = cfg

	return runtimeConfig.Server.GinMode, nil
}

func loadRuntimeFileConfig(path string) (RuntimeFileConfig, error) {
	var cfg RuntimeFileConfig

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, fmt.Errorf("config file %s is required", path)
		}
		return cfg, fmt.Errorf("failed to stat config file %s: %w", path, err)
	}
	if info.IsDir() {
		return cfg, fmt.Errorf("config file path %s points to a directory", path)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("failed to read config file %s: %w", path, err)
	}
	if err := yaml.Unmarshal(content, &cfg); err != nil {
		return cfg, fmt.Errorf("failed to parse config file %s: %w", path, err)
	}

	return cfg, nil
}

func applyRuntimeDefaults(cfg *RuntimeFileConfig) {
	cfg.Server.GinMode = normalizeOrDefault(cfg.Server.GinMode, defaultGinMode)
	if cfg.Server.Port <= 0 {
		cfg.Server.Port = defaultPort
	}

	cfg.Database.Path = normalizeOrDefault(cfg.Database.Path, defaultDBPath)
	cfg.Container.SSLCertPath = normalizeOrDefault(cfg.Container.SSLCertPath, defaultContainerSSLCertPath)
	cfg.Container.SSLKeyPath = normalizeOrDefault(cfg.Container.SSLKeyPath, defaultContainerSSLKeyPath)
}

func GetRuntimeConfig() RuntimeFileConfig {
	return runtimeConfig
}

func InsecureSecuritySettings() []InsecureSecuritySetting {
	return FindInsecureSecuritySettings(runtimeConfig)
}

func FindInsecureSecuritySettings(cfg RuntimeFileConfig) []InsecureSecuritySetting {
	checks := []struct {
		path         string
		label        string
		value        string
		exampleValue string
	}{
		{"security.encryption_key", "数据加密密钥", cfg.Security.EncryptionKey, exampleEncryptionKey},
		{"security.jwt_secret", "JWT 签名密钥", cfg.Security.JWTSecret, exampleJWTSecret},
		{"security.container_secret", "容器共享密钥", cfg.Security.ContainerSecret, exampleContainerSecret},
	}

	var issues []InsecureSecuritySetting
	for _, check := range checks {
		value := strings.TrimSpace(check.value)
		if value == "" {
			issues = append(issues, InsecureSecuritySetting{
				Path:   check.path,
				Label:  check.label,
				Reason: "当前为空，无法提供稳定的安全保护",
			})
			continue
		}
		if value == check.exampleValue {
			issues = append(issues, InsecureSecuritySetting{
				Path:   check.path,
				Label:  check.label,
				Reason: "仍在使用 config.yaml.example 中的示例默认值",
			})
		}
	}

	token := strings.TrimSpace(cfg.Security.AgentRegistrationToken)
	if token == exampleAgentToken {
		issues = append(issues, InsecureSecuritySetting{
			Path:   "security.agent_registration_token",
			Label:  "Agent 注册令牌",
			Reason: "仍在使用 config.yaml.example 中的示例默认值",
		})
	}

	return issues
}

func GinMode() string {
	return runtimeConfig.Server.GinMode
}

func ServerPort() int {
	return runtimeConfig.Server.Port
}

func DatabasePath() string {
	return runtimeConfig.Database.Path
}

func EncryptionKey() string {
	return strings.TrimSpace(runtimeConfig.Security.EncryptionKey)
}

func JWTSecret() string {
	return strings.TrimSpace(runtimeConfig.Security.JWTSecret)
}

func ContainerSecret() string {
	return strings.TrimSpace(runtimeConfig.Security.ContainerSecret)
}

func AgentRegistrationToken() string {
	token := strings.TrimSpace(runtimeConfig.Security.AgentRegistrationToken)
	if token != "" {
		return token
	}
	return ContainerSecret()
}

func PlatformBasicAuthEnabled() bool {
	return runtimeConfig.Security.PlatformBasicAuthEnabled
}

func ContainerSSLCertPath() string {
	return runtimeConfig.Container.SSLCertPath
}

func ContainerSSLKeyPath() string {
	return runtimeConfig.Container.SSLKeyPath
}

func PublicBaseURL() string {
	return strings.TrimRight(strings.TrimSpace(runtimeConfig.PublicBaseURL), "/")
}

func MailTrackingEnabled() bool {
	if runtimeConfig.MailTracking.Enabled == nil {
		return true
	}
	return *runtimeConfig.MailTracking.Enabled
}

func MailTrackingSecret() string {
	secret := strings.TrimSpace(runtimeConfig.MailTracking.Secret)
	if secret != "" {
		return secret
	}
	// Fall back to JWT secret so local setups work without extra config.
	return JWTSecret()
}

func MailTrackingAllowRedirectHosts() []string {
	out := make([]string, 0, len(runtimeConfig.MailTracking.AllowRedirectHosts))
	for _, host := range runtimeConfig.MailTracking.AllowRedirectHosts {
		host = strings.ToLower(strings.TrimSpace(host))
		if host != "" {
			out = append(out, host)
		}
	}
	return out
}

func MailTrackingClickPath() string {
	return mailTrackingAPIPath(runtimeConfig.MailTracking.ClickPath, "click")
}

func MailTrackingOpenPath() string {
	return mailTrackingAPIPath(runtimeConfig.MailTracking.OpenPath, "open")
}

func mailTrackingAPIPath(configured, kind string) string {
	configured = strings.TrimSpace(configured)
	if configured != "" {
		return NormalizeMailTrackingPath(configured)
	}
	return DeriveMailTrackingAPIPath(MailTrackingSecret(), kind)
}

// NormalizeMailTrackingPath ensures a leading slash and no trailing slash.
func NormalizeMailTrackingPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return path
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if len(path) > 1 {
		path = strings.TrimRight(path, "/")
	}
	return path
}

// DeriveMailTrackingAPIPath builds an opaque /api/<slug> path from a secret.
func DeriveMailTrackingAPIPath(secret, kind string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(secret) + "|mail-track|" + kind))
	slug := strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(sum[:13]))
	if len(slug) > 20 {
		slug = slug[:20]
	}
	return "/api/" + slug
}

func normalizeOrDefault(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	if filepath.IsAbs(value) {
		return value
	}
	return filepath.Clean(value)
}
