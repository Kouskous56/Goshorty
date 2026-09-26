package storage

import (
	"testing"
	"time"

	"goshorty/models"
)

func TestMemoryListURLsPagination(t *testing.T) {
	store := NewStorage()
	defer store.Stop()

	base := time.Now().Add(-time.Hour).UTC()
	mk := func(code string, created time.Time, owner string) *models.URLData {
		return &models.URLData{
			ID:          code + "-id",
			ShortCode:   code,
			OriginalURL: "https://example.com/" + code,
			ExpiresIn:   "24h",
			ExpiresAt:   time.Now().Add(24 * time.Hour),
			CreatedAt:   created,
			CreatedBy:   owner,
		}
	}
	for _, u := range []*models.URLData{
		mk("b", base.Add(30*time.Second), "u1"),
		mk("a", base.Add(30*time.Second), "u1"),
		mk("c", base.Add(20*time.Second), "u1"),
		mk("d", base.Add(10*time.Second), "u1"),
		mk("x", base.Add(40*time.Second), "u2"),
	} {
		if err := store.SetIfAbsent(u.ShortCode, u); err != nil {
			t.Fatal(err)
		}
	}

	codes := func(items []*models.URLData) string {
		out := ""
		for _, u := range items {
			out += u.ShortCode
		}
		return out
	}

	// User-scoped first page: only u1's URLs, ordered a, b (timestamp tie,
	// code ASC), total excludes the foreign URL.
	res, err := store.ListURLs("u1", models.RoleUser, models.URLCursor{}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if got := codes(res.Items); got != "ab" {
		t.Fatalf("page 1 = %q, want ab", got)
	}
	if res.Total != 4 || !res.HasMore {
		t.Fatalf("page 1 metadata: total=%d hasMore=%v", res.Total, res.HasMore)
	}

	// Second page via the keyset position of the last returned item.
	last := res.Items[len(res.Items)-1]
	page2, err := store.ListURLs("u1", models.RoleUser, models.URLCursor{
		CreatedAtUnixNano: last.CreatedAt.UnixNano(),
		ShortCode:         last.ShortCode,
	}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if got := codes(page2.Items); got != "cd" {
		t.Fatalf("page 2 = %q, want cd", got)
	}
	if page2.HasMore || page2.Total != 4 {
		t.Fatalf("page 2 metadata: total=%d hasMore=%v", page2.Total, page2.HasMore)
	}

	// Admin sees every URL, including the foreign one.
	resA, err := store.ListURLs("", models.RoleAdmin, models.URLCursor{}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if got := codes(resA.Items); got != "xa" {
		t.Fatalf("admin page = %q, want xa", got)
	}
	if resA.Total != 5 {
		t.Fatalf("admin total = %d, want 5", resA.Total)
	}

	// A cursor past the end returns an empty page but keeps the total.
	past := models.URLCursor{CreatedAtUnixNano: base.Add(-time.Minute).UnixNano(), ShortCode: ""}
	resEnd, err := store.ListURLs("u1", models.RoleUser, past, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(resEnd.Items) != 0 || resEnd.HasMore || resEnd.Total != 4 {
		t.Fatalf("past-end page: %d items, total=%d hasMore=%v", len(resEnd.Items), resEnd.Total, resEnd.HasMore)
	}
}

func TestMemoryListURLsExcludesExpired(t *testing.T) {
	store := NewStorage()
	defer store.Stop()

	err := store.SetIfAbsent("gone", &models.URLData{
		ID:          "gone-id",
		ShortCode:   "gone",
		OriginalURL: "https://example.com",
		ExpiresIn:   "5m",
		ExpiresAt:   time.Now().Add(-time.Minute),
		CreatedAt:   time.Now(),
		CreatedBy:   "u1",
	})
	if err != nil {
		t.Fatal(err)
	}

	res, err := store.ListURLs("u1", models.RoleUser, models.URLCursor{}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 0 || len(res.Items) != 0 {
		t.Fatalf("expired URL counted: total=%d items=%d", res.Total, len(res.Items))
	}
}

func TestMemoryListUsersPagination(t *testing.T) {
	us, err := NewUserStorage("storage-admin-pw", "admin@test.local")
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range []struct{ name, email string }{
		{"bravo", "bravo@test.local"},
		{"zeta", "zeta@test.local"},
		{"alpha", "alpha@test.local"},
	} {
		if _, err := us.CreateUser(u.name, "password-"+u.name, u.email); err != nil {
			t.Fatal(err)
		}
	}

	names := func(items []*models.User) []string {
		out := make([]string, 0, len(items))
		for _, u := range items {
			out = append(out, u.Username)
		}
		return out
	}

	page1, err := us.ListUsers(models.UserCursor{}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if got := names(page1.Items); len(got) != 2 || got[0] != "admin" || got[1] != "alpha" {
		t.Fatalf("page 1 = %v, want [admin alpha]", got)
	}
	if page1.Total != 4 || !page1.HasMore {
		t.Fatalf("page 1 metadata: total=%d hasMore=%v", page1.Total, page1.HasMore)
	}

	page2, err := us.ListUsers(models.UserCursor{Username: "alpha"}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if got := names(page2.Items); len(got) != 2 || got[0] != "bravo" || got[1] != "zeta" {
		t.Fatalf("page 2 = %v, want [bravo zeta]", got)
	}
	if page2.HasMore || page2.Total != 4 {
		t.Fatalf("page 2 metadata: total=%d hasMore=%v", page2.Total, page2.HasMore)
	}

	page3, err := us.ListUsers(models.UserCursor{Username: "zeta"}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(page3.Items) != 0 || page3.HasMore || page3.Total != 4 {
		t.Fatalf("past-end page: %d items, total=%d hasMore=%v", len(page3.Items), page3.Total, page3.HasMore)
	}
}
