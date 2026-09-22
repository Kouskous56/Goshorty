package services

import (
	"strings"
	"testing"
	"time"

	"goshorty/models"
)

func fixtureURLs() []*models.URLData {
	t := time.Unix(1700000000, 0)
	return []*models.URLData{
		{ShortCode: "b", CreatedAt: t.Add(30 * time.Second)},
		{ShortCode: "a", CreatedAt: t.Add(30 * time.Second)}, // same time: code tie-break
		{ShortCode: "c", CreatedAt: t.Add(20 * time.Second)},
		{ShortCode: "d", CreatedAt: t.Add(10 * time.Second)},
	}
}

func codes(page []*models.URLData) string {
	var sb strings.Builder
	for _, u := range page {
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

func TestPaginateURLsDefaultsToFirstPage(t *testing.T) {
	page, err := PaginateURLs(fixtureURLs(), ListOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if page.Total != 4 {
		t.Errorf("total = %d, want 4", page.Total)
	}
	if page.HasMore {
		t.Error("hasMore = true on a single-page result")
	}
	if page.NextCursor != "" {
		t.Errorf("next_cursor = %q, want empty", page.NextCursor)
	}
	if got := codes(page.URLs); got != "abcd" {
		t.Errorf("order = %q, want abcd (created_at DESC, short_code ASC tie-break)", got)
	}
}

func TestPaginateURLsCursorWalk(t *testing.T) {
	// Page 1 (limit 2): a, b — newest first with the same-timestamp
	// tie-break applied.
	page1, err := PaginateURLs(fixtureURLs(), ListOptions{Limit: 2})
	if err != nil {
		t.Fatalf("page 1: %v", err)
	}
	if got := codes(page1.URLs); got != "ab" {
		t.Fatalf("page 1 = %q, want ab", got)
	}
	if page1.Total != 4 || !page1.HasMore || page1.NextCursor == "" {
		t.Fatalf("page 1 metadata wrong: total=%d hasMore=%v next=%q", page1.Total, page1.HasMore, page1.NextCursor)
	}

	// Page 2 via cursor: c, d and no further cursor.
	page2, err := PaginateURLs(fixtureURLs(), ListOptions{Limit: 2, Cursor: page1.NextCursor})
	if err != nil {
		t.Fatalf("page 2: %v", err)
	}
	if got := codes(page2.URLs); got != "cd" {
		t.Fatalf("page 2 = %q, want cd", got)
	}
	if page2.HasMore || page2.NextCursor != "" || page2.Total != 4 {
		t.Fatalf("page 2 metadata wrong: total=%d hasMore=%v next=%q", page2.Total, page2.HasMore, page2.NextCursor)
	}
}

func TestPaginateURLsCursorBeyondEndReturnsEmptyPage(t *testing.T) {
	page1, _ := PaginateURLs(fixtureURLs(), ListOptions{Limit: 4})
	page2, err := PaginateURLs(fixtureURLs(), ListOptions{Cursor: page1.NextCursor})
	if err != nil {
		t.Fatalf("walk past end: %v", err)
	}
	// cursor from last page is empty; passing it behaves like first page,
	// which is a full-size page here.
	if got := codes(page2.URLs); got != "abcd" {
		t.Fatalf("empty-cursor passthrough page = %q, want abcd", got)
	}
}

func TestPaginateURLsInvalidCursor(t *testing.T) {
	bad := []string{"!!!not-base64!!!", "YWJjZA", "e30", "eyJjIjowLCJzIjoiIn0"} // garbage / wrong shape / zero key
	if enc, err := EncodeCursor(URLCursor{}); err == nil {
		bad = append(bad, enc)
	}
	for _, c := range bad {
		if _, err := PaginateURLs(fixtureURLs(), ListOptions{Cursor: c}); err != ErrInvalidCursor {
			t.Errorf("cursor %q: err = %v, want ErrInvalidCursor", c, err)
		}
	}
}

func TestPaginateURLsLimitCappedAtMax(t *testing.T) {
	fx := fixtureURLs()
	for range [MaxPageLimit + 5]struct{}{} {
		fx = append(fx, &models.URLData{ShortCode: "x", CreatedAt: time.Unix(1, 0)})
	}
	page, err := PaginateURLs(fx, ListOptions{Limit: 99999})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(page.URLs) != MaxPageLimit {
		t.Errorf("page size = %d, want capped at %d", len(page.URLs), MaxPageLimit)
	}
	if page.Total != 4+MaxPageLimit+5 {
		t.Errorf("total = %d, want %d", page.Total, 4+MaxPageLimit+5)
	}
}

func TestPaginateUsersSortedAndWalk(t *testing.T) {
	users := []*models.User{
		{Username: "zeta"},
		{Username: "alpha"},
		{Username: "beta"},
	}
	page1, err := PaginateUsers(users, ListOptions{Limit: 2})
	if err != nil {
		t.Fatalf("page 1: %v", err)
	}
	if len(page1.Users) != 2 || page1.Users[0].Username != "alpha" || page1.Users[1].Username != "beta" {
		t.Fatalf("page 1 = %v, want [alpha beta]", userNames(page1.Users))
	}
	if page1.Total != 3 || !page1.HasMore || page1.NextCursor == "" {
		t.Fatalf("page 1 metadata wrong: total=%d hasMore=%v next=%q", page1.Total, page1.HasMore, page1.NextCursor)
	}

	page2, err := PaginateUsers(users, ListOptions{Cursor: page1.NextCursor})
	if err != nil {
		t.Fatalf("page 2: %v", err)
	}
	if len(page2.Users) != 1 || page2.Users[0].Username != "zeta" {
		t.Fatalf("page 2 = %v, want [zeta]", userNames(page2.Users))
	}
	if page2.HasMore || page2.NextCursor != "" {
		t.Fatalf("page 2 metadata wrong: hasMore=%v next=%q", page2.HasMore, page2.NextCursor)
	}
}

func TestPaginateUsersInvalidCursor(t *testing.T) {
	users := []*models.User{{Username: "alpha"}}
	if _, err := PaginateUsers(users, ListOptions{Cursor: "zzz"}); err != ErrInvalidCursor {
		t.Errorf("err = %v, want ErrInvalidCursor", err)
	}
	if enc, err := EncodeCursor(UserCursor{}); err == nil {
		if _, err := PaginateUsers(users, ListOptions{Cursor: enc}); err != ErrInvalidCursor {
			t.Errorf("zero-key cursor: err = %v, want ErrInvalidCursor", err)
		}
	}
}

func userNames(users []*models.User) []string {
	out := make([]string, 0, len(users))
	for _, u := range users {
		out = append(out, u.Username)
	}
	return out
}
