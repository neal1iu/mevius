package domain

import "testing"

func TestPageSpecRequiresSourceRepository(t *testing.T) {
	if err := (PageSpec{Name: "site"}).Validate(); err == nil {
		t.Fatal("expected source repository validation error")
	}
	if err := (PageSpec{Name: "site", SourceRepoInstanceID: "repo"}).Validate(); err != nil {
		t.Fatal(err)
	}
}
func TestCanonicalResourceKinds(t *testing.T) {
	got := []ResourceKind{ResourceKindGitRepo, ResourceKindCIPipeline, ResourceKindPage, ResourceKindServerlessService, ResourceKindDNSZone}
	want := []ResourceKind{"git_repo", "ci_pipeline", "page", "serverless_service", "dns_zone"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("kind %d = %q", i, got[i])
		}
	}
}
