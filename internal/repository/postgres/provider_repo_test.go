package postgres

import (
	"context"
	"testing"

	"polaris/internal/repository"
)

func TestProviderCreatePersistsOAuthConfig(t *testing.T) {
	client, _ := integrationClient(t)
	institutionID := createInstitutionFixture(t, client, "provider-create-test")
	providers := NewProviderRepository(client)
	item := &repository.Provider{
		InstitutionID:      institutionID,
		Name:               "corp-sso",
		DisplayName:        "Corp SSO",
		Category:           "oauth",
		Type:               "oidc",
		ClientID:           "corp-client",
		ClientSecret:       "corp-secret",
		IssuerURL:          "https://idp.example.test",
		Scopes:             []string{"openid", "email"},
		EnablePKCE:         true,
		AutoCreate:         false,
		MatchType:          "system",
		MatchLocalField:    "email",
		MatchProviderField: "email",
		RedirectURI:        "https://polaris.example.test/sso/callback",
		State:              "active",
	}

	id, err := providers.Create(context.Background(), item)
	if err != nil {
		t.Fatalf("create oauth provider: %v", err)
	}
	if id == 0 || item.ID != id {
		t.Fatalf("provider id = %d, entity id = %d", id, item.ID)
	}

	persisted, err := providers.GetByID(context.Background(), id)
	if err != nil {
		t.Fatalf("reload oauth provider: %v", err)
	}
	if persisted.InstitutionID != institutionID ||
		persisted.ClientID != item.ClientID ||
		persisted.ClientSecret != item.ClientSecret ||
		persisted.IssuerURL != item.IssuerURL ||
		!persisted.EnablePKCE {
		t.Fatalf("oauth config was not persisted: %+v", persisted)
	}
}

func TestProviderCreatePersistsIdentityLinkMatchSource(t *testing.T) {
	client, _ := integrationClient(t)
	institutionID := createInstitutionFixture(t, client, "provider-identity-match")
	ctx := context.Background()

	var syncerID int64
	if err := client.QueryRow(ctx, `INSERT INTO syncers (name, type, institution_id)
		VALUES ('hr-source', 'http', $1) RETURNING id`, institutionID).Scan(&syncerID); err != nil {
		t.Fatalf("create syncer: %v", err)
	}

	providers := NewProviderRepository(client)
	item := &repository.Provider{
		InstitutionID:      institutionID,
		Name:               "corp-sso",
		DisplayName:        "Corp SSO",
		Category:           "oauth",
		Type:               "oidc",
		ClientID:           "corp-client",
		ClientSecret:       "corp-secret",
		IssuerURL:          "https://idp.example.test",
		Scopes:             []string{"openid"},
		MatchType:          "identity_link",
		MatchSyncerID:      syncerID,
		MatchSyncerField:   "external_id",
		MatchProviderField: "unionid",
		State:              "active",
	}
	id, err := providers.Create(ctx, item)
	if err != nil {
		t.Fatalf("create identity-link provider: %v", err)
	}
	got, err := providers.GetByID(ctx, id)
	if err != nil {
		t.Fatalf("reload identity-link provider: %v", err)
	}
	if got.MatchType != "identity_link" || got.MatchSyncerID != syncerID ||
		got.MatchSyncerField != "external_id" || got.MatchProviderField != "unionid" {
		t.Fatalf("identity-link match source not persisted: %+v", got)
	}
	if err := client.Exec(ctx, `UPDATE provider_oauth SET match_local_field='email' WHERE provider_id=$1`, id); err == nil {
		t.Fatal("mixing system and identity-link match fields must violate the CHECK constraint")
	}

	if err := client.Exec(ctx, `DELETE FROM syncers WHERE id=$1`, syncerID); err == nil {
		t.Fatal("deleting a referenced identity-link source must be rejected")
	}
}
