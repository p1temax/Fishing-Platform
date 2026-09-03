package config

import "testing"

func TestFindInsecureSecuritySettings(t *testing.T) {
	var cfg RuntimeFileConfig
	cfg.Security.EncryptionKey = exampleEncryptionKey
	cfg.Security.JWTSecret = "   "
	cfg.Security.ContainerSecret = exampleContainerSecret

	issues := FindInsecureSecuritySettings(cfg)
	if len(issues) != 3 {
		t.Fatalf("FindInsecureSecuritySettings() returned %d issues, want 3", len(issues))
	}

	wantPaths := map[string]bool{
		"security.encryption_key":   false,
		"security.jwt_secret":       false,
		"security.container_secret": false,
	}
	for _, issue := range issues {
		if _, ok := wantPaths[issue.Path]; !ok {
			t.Fatalf("unexpected insecure setting path %q", issue.Path)
		}
		wantPaths[issue.Path] = true
	}
	for path, found := range wantPaths {
		if !found {
			t.Fatalf("expected insecure setting path %q", path)
		}
	}
}

func TestFindInsecureSecuritySettingsAcceptsCustomSecrets(t *testing.T) {
	var cfg RuntimeFileConfig
	cfg.Security.EncryptionKey = "custom-encryption-key-with-enough-random-text"
	cfg.Security.JWTSecret = "custom-jwt-secret-with-enough-random-text"
	cfg.Security.ContainerSecret = "custom-container-secret-with-enough-random-text"

	if issues := FindInsecureSecuritySettings(cfg); len(issues) != 0 {
		t.Fatalf("FindInsecureSecuritySettings() returned %d issues, want 0", len(issues))
	}
}

func TestFindInsecureSecuritySettingsFlagsExampleAgentRegistrationToken(t *testing.T) {
	var cfg RuntimeFileConfig
	cfg.Security.EncryptionKey = "custom-encryption-key-with-enough-random-text"
	cfg.Security.JWTSecret = "custom-jwt-secret-with-enough-random-text"
	cfg.Security.ContainerSecret = "custom-container-secret-with-enough-random-text"
	cfg.Security.AgentRegistrationToken = exampleAgentToken

	issues := FindInsecureSecuritySettings(cfg)
	if len(issues) != 1 {
		t.Fatalf("FindInsecureSecuritySettings() returned %d issues, want 1", len(issues))
	}
	if issues[0].Path != "security.agent_registration_token" {
		t.Fatalf("unexpected insecure setting path %q", issues[0].Path)
	}
}
