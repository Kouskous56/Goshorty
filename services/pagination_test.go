package services

import (
	"strings"
	"testing"
	"time"

	"goshorty/config"
	"goshorty/models"
	"goshorty/storage"
)

// seedMemoryURLs populates a fresh in-memory store with the given URLs.
func seedMemoryURLs(t *testing.T, urls []*models.URLData) *storage.Storage {
	t.Helper()
	store := storage.NewStorage()
	t.Cleanup(store.Stop)
	for _, u := range urls {
		if err := store.SetIfAbsent(u.ShortCode, u); err != nil {
			t.Fatalf("seed %s: %v", u.ShortCode, err)
		}
	}
	return store
}

func memoryURLService(t *testing.T, urls []*models.URLData) *URLService {
	t.Helper()
	return NewURLService(seedMemoryURLs(t, urls), config.NewConfig())
}

// fixtureURLData returns URLs ordered (created_at DESC, short_code ASC):
// x, a, b, c, d — where a/b share a timestamp (code tie-break) and x belongs
// to a different owner to exercise ownership scoping.
func fixtureURLData() []*models.URLData {
	base := time.Unix(1700000000, 0).UTC()
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
	return []*models.URLData{
		mk("x", base.Add(40*time.Second), "other-user"),
		mk("b", base.Add(30*time.Second), "owner-user"),
		mk("a", base.Add(30*time.Second), "owner-user"),
		mk("c", base.Add(20*time.Second), "owner-user"),
		mk("d", base.Add(10*time.Second), "owner-user"),
	}
}

func pageCodes(page URLPage) string {
	var sb strings.Builder
	for _, u := range page.URLs {
		sb.WriteString(u.ShortCode)
	}
	return sb.String()
}

func TestNormalizeLimit(t *testing.T) {
	cases := []struct{ in, want int }{
		{0, DefaultPageLimit},
		{-5, DefaultPageLimit},
		{50, 50},
		{1, 1},
		{200, 200},
		{2000, MaxPageLimit},
	}
	for _, c := range cases {
		if got := NormalizeLimit(c.in); got != c.want {
			t.Errorf("NormalizeLimit(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestListURLsFirstPageOrderAndMetadata(t *testing.T) {
	svc := memoryURLService(t, fixtureURLData())
	page, err := svc.ListURLs("owner-user", models.RoleUser, ListOptions{Limit: 2})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if got := pageCodes(page); got != "ab" {
		t.Fatalf("page = %q, want ab (created_at DESC, short_code ASC tie-break)", got)
	}
	if page.Total != 4 {
		t.Errorf("total = %d, want 4 (other-user URL excluded)", page.Total)
	}
	if !page.HasMore || page.NextCursor == "" {
		t.Fatalf("page metadata wrong: hasMore=%v next=%q", page.HasMore, page.NextCursor)
	}
}

func TestListURLsCursorWalk(t *testing.T) {
	svc := memoryURLService(t, fixtureURLData())
	page1, err := svc.ListURLs("owner-user", models.RoleUser, ListOptions{Limit: 2})
	if err != nil {
		t.Fatalf("page 1: %v", err)
	}
	if got := pageCodes(page1); got != "ab" {
		t.Fatalf("page 1 = %q, want ab", got)
	}

	page2, err := svc.ListURLs("owner-user", models.RoleUser, ListOptions{Limit: 2, Cursor: page1.NextCursor})
	if err != nil {
		t.Fatalf("page 2: %v", err)
	}
	if got := pageCodes(page2); got != "cd" {
		t.Fatalf("page 2 = %q, want cd", got)
	}
	if page2.HasMore || page2.NextCursor != "" {
		t.Fatalf("page 2 metadata wrong: hasMore=%v next=%q", page2.HasMore, page2.NextCursor)
	}
	if page2.Total != 4 {
		t.Errorf("page 2 total = %d, want 4", page2.Total)
	}
}

func TestListURLsAdminSeesEverything(t *testing.T) {
	svc := memoryURLService(t, fixtureURLData())
	page, err := svc.ListURLs("", models.RoleAdmin, ListOptions{Limit: 2})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if got := pageCodes(page); got != "xa" {
		t.Fatalf("admin page = %q, want xa (newest first)", got)
	}
	if page.Total != 5 {
		t.Errorf("admin total = %d, want 5", page.Total)
	}
}

func TestListURLsInvalidCursor(t *testing.T) {
	svc := memoryURLService(t, fixtureURLData())
	bad := []string{"!!!not-base64!!!", "YWJjZA", "e30"}
	if enc, err := models.EncodeCursor(models.URLCursor{}); err == nil {
		bad = append(bad, enc)
	}
	for _, c := range bad {
		if _, err := svc.ListURLs("owner-user", models.RoleUser, ListOptions{Cursor: c}); err != ErrInvalidCursor {
			t.Errorf("cursor %q: err = %v, want ErrInvalidCursor", c, err)
		}
	}
}

func TestListURLsCursorPastEndReturnsEmptyPage(t *testing.T) {
	svc := memoryURLService(t, fixtureURLData())
	all, err := svc.ListURLs("owner-user", models.RoleUser, ListOptions{Limit: 10})
	if err != nil || len(all.URLs) != 4 {
		t.Fatalf("full page = %d items, err=%v", len(all.URLs), err)
	}
	last := all.URLs[len(all.URLs)-1]
	past, err := models.EncodeCursor(models.URLCursor{
		CreatedAtUnixNano: last.CreatedAt.UnixNano(),
		ShortCode:         last.ShortCode,
	})
	if err != nil {
		t.Fatalf("encode cursor: %v", err)
	}
	page, err := svc.ListURLs("owner-user", models.RoleUser, ListOptions{Cursor: past})
	if err != nil {
		t.Fatalf("cursor past end: %v", err)
	}
	if len(page.URLs) != 0 || page.HasMore {
		t.Fatalf("past-end page: %d items, hasMore=%v — want empty", len(page.URLs), page.HasMore)
	}
	if page.Total != 4 {
		t.Errorf("past-end total = %d, want 4", page.Total)
	}
}
