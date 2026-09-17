package service

import (
	"encoding/json"
	"testing"

	"mevius/internal/domain"
)

func TestSlotConfigValidateRepo(t *testing.T) {
	tests := []struct {
		name    string
		config  json.RawMessage
		wantErr bool
	}{
		{name: "valid repo", config: json.RawMessage(`{"name":"my-repo"}`), wantErr: false},
		{name: "valid repo with all fields", config: json.RawMessage(`{"name":"my-repo","private":true,"description":"desc","workflow_id":"ci.yml","workflow_ref":"main"}`), wantErr: false},
		{name: "invalid repo missing name", config: json.RawMessage(`{}`), wantErr: true},
		{name: "invalid repo empty name", config: json.RawMessage(`{"name":""}`), wantErr: true},
		{name: "invalid repo bad json", config: json.RawMessage(`{bad`), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSlotConfig(domain.SlotRoleSource, tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateSlotConfig(repo, %s) error = %v, wantErr = %v", string(tt.config), err, tt.wantErr)
			}
		})
	}
}

func TestSlotConfigValidateCompute(t *testing.T) {
	tests := []struct {
		name    string
		config  json.RawMessage
		wantErr bool
	}{
		{name: "valid compute", config: json.RawMessage(`{"name":"my-worker"}`), wantErr: false},
		{name: "valid compute with date", config: json.RawMessage(`{"name":"my-worker","compatibility_date":"2024-01-01"}`), wantErr: false},
		{name: "invalid compute missing name", config: json.RawMessage(`{}`), wantErr: true},
		{name: "invalid compute bad json", config: json.RawMessage(`{bad`), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSlotConfig(domain.SlotRoleBackend, tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateSlotConfig(compute, %s) error = %v, wantErr = %v", string(tt.config), err, tt.wantErr)
			}
		})
	}
}

func TestSlotConfigValidateStaticSite(t *testing.T) {
	tests := []struct {
		name    string
		config  json.RawMessage
		wantErr bool
	}{
		{name: "valid static-site", config: json.RawMessage(`{"name":"my-site"}`), wantErr: false},
		{name: "valid static-site with all fields", config: json.RawMessage(`{"name":"my-site","framework":"react","build_command":"npm run build","output_dir":"dist","root_dir":"/","production_branch":"main"}`), wantErr: false},
		{name: "invalid static-site missing name", config: json.RawMessage(`{}`), wantErr: true},
		{name: "invalid static-site bad json", config: json.RawMessage(`{bad`), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSlotConfig(domain.SlotRoleFrontend, tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateSlotConfig(static-site, %s) error = %v, wantErr = %v", string(tt.config), err, tt.wantErr)
			}
		})
	}
}

func TestSlotConfigValidateDNSDomain(t *testing.T) {
	tests := []struct {
		name   string
		config json.RawMessage
	}{
		{name: "empty config", config: json.RawMessage(`{}`)},
		{name: "null config", config: json.RawMessage(`null`)},
		{name: "with extra fields", config: json.RawMessage(`{"zone":"example.com"}`)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSlotConfig(domain.SlotRoleDNS, tt.config)
			if err != nil {
				t.Errorf("validateSlotConfig(dns-domain, %s) error = %v, want nil", string(tt.config), err)
			}
		})
	}
}

func TestSlotConfigUnknownType(t *testing.T) {
	err := validateSlotConfig("unknown-type", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for unknown slot type")
	}
}