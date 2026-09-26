package services

import (
	"errors"
	"testing"

	"goshorty/config"
	"goshorty/models"
	"goshorty/storage"
)

// bolaServiceFixture seeds one URL owned by alice and returns the service and
// the two identities used in cross-user assertions.
func bolaServiceFixture(t *testing.T) (*URLService, string, string) {
	t.Helper()
	store := storage.NewStorage()
	t.Cleanup(store.Stop)
	svc := NewURLService(store, config.NewConfig())
	if _, err := svc.CreateShortURL(&models.ShortenRequest{
		URL:        "https://example.com/alice-private",
		CustomCode: "alice1",
	}, "user-alice"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return svc, "user-alice", "user-bob"
}

// TestDeleteURLRejectsNonOwnerSilently proves the delete path follows the
// same opaque model as reads: a non-owner gets ErrForbidden (mapped to 404 by
// the handler) and the URL is untouched, so a non-owner cannot distinguish a
// foreign URL from a missing one.
func TestDeleteURLRejectsNonOwnerSilently(t *testing.T) {
	svc, alice, bob := bolaServiceFixture(t)

	if err := svc.DeleteURL("alice1", bob, models.RoleUser); !errors.Is(err, ErrForbidden) {
		t.Fatalf("cross-user delete: err = %v, want ErrForbidden", err)
	}
	// The URL is still intact and readable by the owner.
	if _, err := svc.GetURLInfo("alice1", alice, models.RoleUser); err != nil {
		t.Fatalf("URL vanished after a denied delete: %v", err)
	}

	// Owner (or admin) can delete it, and afterwards it is gone for everyone.
	if err := svc.DeleteURL("alice1", alice, models.RoleUser); err != nil {
		t.Fatalf("owner delete: %v", err)
	}
	if _, err := svc.GetURLInfo("alice1", alice, models.RoleUser); !errors.Is(err, ErrURLNotFound) {
		t.Fatalf("after delete: err = %v, want ErrURLNotFound", err)
	}
}

// TestRedirectIsTTLGatedNotOwnerGated records the intentional exception: the
// public redirect (/r/:code) resolves any active URL for anonymous callers.
func TestRedirectIsTTLGatedNotOwnerGated(t *testing.T) {
	svc, _, _ := bolaServiceFixture(t)
	got, err := svc.GetOriginalURL("alice1")
	if err != nil {
		t.Fatalf("anonymous redirect: %v", err)
	}
	if got != "https://example.com/alice-private" {
		t.Fatalf("unexpected destination: %q", got)
	}
}

// TestListScopingComplementsGetInfo proves the list surface hides foreign
// rows for users while the redirect stays public.
func TestListScopingComplementsGetInfo(t *testing.T) {
	svc, alice, bob := bolaServiceFixture(t)

	alicePage, err := svc.ListURLs(alice, models.RoleUser, ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	bobPage, err := svc.ListURLs(bob, models.RoleUser, ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if alicePage.Total != 1 || bobPage.Total != 0 {
		t.Fatalf("list scoping: alice total=%d bob total=%d, want 1 and 0", alicePage.Total, bobPage.Total)
	}
}
