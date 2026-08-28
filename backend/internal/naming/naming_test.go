package naming

import "testing"

func TestUpstreamNameUsesDomainBrandForDefaultSiteTitle(t *testing.T) {
	if got := UpstreamName("New API", "https://api.aiyxgaw.com"); got != "aiyxgaw" {
		t.Fatalf("UpstreamName()=%q want aiyxgaw", got)
	}
}

func TestUpstreamNameKeepsDistinctSiteTitle(t *testing.T) {
	if got := UpstreamName("Anc1ent API", "https://api.anc1ent.top"); got != "Anc1ent API" {
		t.Fatalf("UpstreamName()=%q want Anc1ent API", got)
	}
}

func TestAccountNameUsesNormalizedUpstreamName(t *testing.T) {
	if got := AccountName("Anc1ent API", "https://api.anc1ent.top", "1"); got != "Anc1ent API-1" {
		t.Fatalf("AccountName()=%q want Anc1ent API-1", got)
	}
}
