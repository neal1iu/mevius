package domain

import "testing"

func TestRepoConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     RepoConfig
		wantErr bool
	}{
		{name: "valid", cfg: RepoConfig{Name: "my-repo"}, wantErr: false},
		{name: "valid with all fields", cfg: RepoConfig{Name: "my-repo", Private: true, Description: "desc", WorkflowID: "ci.yml", WorkflowRef: "main"}, wantErr: false},
		{name: "missing name", cfg: RepoConfig{}, wantErr: true},
		{name: "empty name", cfg: RepoConfig{Name: ""}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestComputeConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     ComputeConfig
		wantErr bool
	}{
		{name: "valid", cfg: ComputeConfig{Name: "my-worker"}, wantErr: false},
		{name: "valid with date", cfg: ComputeConfig{Name: "my-worker", CompatibilityDate: "2024-01-01"}, wantErr: false},
		{name: "missing name", cfg: ComputeConfig{}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestStaticSiteConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     StaticSiteConfig
		wantErr bool
	}{
		{name: "valid", cfg: StaticSiteConfig{Name: "my-site"}, wantErr: false},
		{name: "valid with all fields", cfg: StaticSiteConfig{Name: "my-site", Framework: "react", BuildCommand: "npm run build", OutputDir: "dist", RootDir: "/", ProductionBranch: "main"}, wantErr: false},
		{name: "missing name", cfg: StaticSiteConfig{}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestDnsDomainConfigValidate(t *testing.T) {
	cfg := DnsDomainConfig{}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() error = %v, want nil", err)
	}
}