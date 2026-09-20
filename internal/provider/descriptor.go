package provider

import "mevius/internal/domain"

var GitHubDescriptor = domain.ProviderDescriptor{
	ID: "github", DisplayName: "GitHub",
	Products: []domain.ProductDescriptor{
		{ID: "github.repositories", ProviderID: "github", DisplayName: "Git Repository", ResourceKind: domain.ResourceKindGitRepo,
			CompatibleRoles: []domain.ResourceRole{domain.ResourceRoleSource},
			Capabilities:    []domain.Capability{domain.CapDiscover, domain.CapInspect, domain.CapCreate, domain.CapDelete},
			Fields:          []domain.FieldSchema{{Name: "name", Label: "Name", Type: "text", Required: true}, {Name: "private", Label: "Private", Type: "boolean"}, {Name: "description", Label: "Description", Type: "text"}}},
		{ID: "github.actions", ProviderID: "github", DisplayName: "Actions Pipeline", ResourceKind: domain.ResourceKindCIPipeline,
			CompatibleRoles: []domain.ResourceRole{domain.ResourceRoleAutomation},
			Capabilities:    []domain.Capability{domain.CapDiscover, domain.CapInspect, domain.CapPipelineTrigger, domain.CapPipelineCancel, domain.CapPipelineRerun, domain.CapPipelineRuns, domain.CapLogs}},
	},
}

var CloudflareDescriptor = domain.ProviderDescriptor{
	ID: "cloudflare", DisplayName: "Cloudflare",
	Products: []domain.ProductDescriptor{
		{ID: "cloudflare.workers", ProviderID: "cloudflare", DisplayName: "Worker", ResourceKind: domain.ResourceKindServerlessService,
			CompatibleRoles: []domain.ResourceRole{domain.ResourceRoleBackend, domain.ResourceRoleInfrastructure},
			Capabilities:    []domain.Capability{domain.CapDiscover, domain.CapInspect}},
		{ID: "cloudflare.pages", ProviderID: "cloudflare", DisplayName: "Pages Project", ResourceKind: domain.ResourceKindPage,
			CompatibleRoles: []domain.ResourceRole{domain.ResourceRoleFrontend, domain.ResourceRoleBackend},
			Capabilities:    []domain.Capability{domain.CapDiscover, domain.CapInspect, domain.CapCreate, domain.CapDelete, domain.CapDeploy, domain.CapLogs},
			Fields:          []domain.FieldSchema{{Name: "name", Label: "Name", Type: "text", Required: true}, {Name: "source_repo_instance_id", Label: "Source repository", Type: "resource", Required: true}, {Name: "production_branch", Label: "Production branch", Type: "text"}}},
		{ID: "cloudflare.dns", ProviderID: "cloudflare", DisplayName: "DNS Zone", ResourceKind: domain.ResourceKindDNSZone,
			CompatibleRoles: []domain.ResourceRole{domain.ResourceRoleInfrastructure},
			Capabilities:    []domain.Capability{domain.CapDiscover, domain.CapInspect, domain.CapDNSManage}},
	},
}

var VercelDescriptor = domain.ProviderDescriptor{
	ID: "vercel", DisplayName: "Vercel",
	Products: []domain.ProductDescriptor{
		{ID: "vercel.projects", ProviderID: "vercel", DisplayName: "Project", ResourceKind: domain.ResourceKindPage,
			CompatibleRoles: []domain.ResourceRole{domain.ResourceRoleFrontend, domain.ResourceRoleBackend},
			Capabilities:    []domain.Capability{domain.CapDiscover, domain.CapInspect, domain.CapCreate, domain.CapDelete, domain.CapDeploy, domain.CapLogs},
			Fields:          []domain.FieldSchema{{Name: "name", Label: "Name", Type: "text", Required: true}, {Name: "source_repo_instance_id", Label: "Source repository", Type: "resource", Required: true}, {Name: "framework", Label: "Framework", Type: "text"}}},
		{ID: "vercel.dns", ProviderID: "vercel", DisplayName: "DNS Zone", ResourceKind: domain.ResourceKindDNSZone,
			CompatibleRoles: []domain.ResourceRole{domain.ResourceRoleInfrastructure},
			Capabilities:    []domain.Capability{domain.CapDiscover, domain.CapInspect, domain.CapDNSManage}},
	},
}

func ProductDescriptor(desc domain.ProviderDescriptor, id domain.ProductID) domain.ProductDescriptor {
	for _, p := range desc.Products {
		if p.ID == id {
			return p
		}
	}
	panic("product descriptor not found: " + string(id))
}
