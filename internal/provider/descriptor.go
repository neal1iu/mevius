package provider

import "mevius/internal/domain"

var GitHubDescriptor = domain.ProviderDescriptor{
	Type: domain.ProviderTypeGitHub,
	Products: []domain.ProductDescriptor{
		{
			ID:           "github.repositories",
			ResourceKind: domain.ResourceKindRepository,
			Roles:        []domain.SlotRole{domain.SlotRoleSource},
			Capabilities: []domain.Capability{domain.CapDiscover, domain.CapInspect, domain.CapCreate, domain.CapDelete},
		},
		{
			ID:           "github.actions",
			ResourceKind: domain.ResourceKindRepository,
			Roles:        []domain.SlotRole{domain.SlotRoleSource},
			Capabilities: []domain.Capability{domain.CapWorkflow, domain.CapDeploy, domain.CapLogs},
		},
	},
}

var CloudflareDescriptor = domain.ProviderDescriptor{
	Type: domain.ProviderTypeCloudflare,
	Products: []domain.ProductDescriptor{
		{
			ID:           "cloudflare.workers",
			ResourceKind: domain.ResourceKindWorker,
			Roles:        []domain.SlotRole{domain.SlotRoleBackend},
			Capabilities: []domain.Capability{domain.CapDiscover, domain.CapInspect, domain.CapCreate, domain.CapDelete},
		},
		{
			ID:           "cloudflare.pages",
			ResourceKind: domain.ResourceKindStaticSite,
			Roles:        []domain.SlotRole{domain.SlotRoleFrontend},
			Capabilities: []domain.Capability{domain.CapDiscover, domain.CapInspect, domain.CapCreate, domain.CapDelete, domain.CapDeploy, domain.CapLogs},
		},
		{
			ID:           "cloudflare.dns",
			ResourceKind: domain.ResourceKindDNSZone,
			Roles:        []domain.SlotRole{domain.SlotRoleDNS},
			Capabilities: []domain.Capability{domain.CapDiscover, domain.CapInspect, domain.CapDNSManage},
		},
	},
}

var VercelDescriptor = domain.ProviderDescriptor{
	Type: domain.ProviderTypeVercel,
	Products: []domain.ProductDescriptor{
		{
			ID:           "vercel.projects",
			ResourceKind: domain.ResourceKindStaticSite,
			Roles:        []domain.SlotRole{domain.SlotRoleFrontend},
			Capabilities: []domain.Capability{domain.CapDiscover, domain.CapInspect, domain.CapCreate, domain.CapDelete, domain.CapDeploy, domain.CapLogs},
		},
		{
			ID:           "vercel.dns",
			ResourceKind: domain.ResourceKindDNSZone,
			Roles:        []domain.SlotRole{domain.SlotRoleDNS},
			Capabilities: []domain.Capability{domain.CapDiscover, domain.CapInspect, domain.CapDNSManage},
		},
	},
}