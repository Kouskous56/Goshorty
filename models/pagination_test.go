package models

import (
	"testing"
	"time"
)

func TestEncodeDecodeURLCursorRoundTrip(t *testing.T) {
	cur := URLCursor{CreatedAtUnixNano: 1700000000000000001, ShortCode: "abc123"}
	enc, err := EncodeCursor(cur)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if enc == "" {
		t.Fatal("encode returned an empty token")
	}
	var got URLCursor
	if err := DecodeCursor(enc, &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got != cur {
		t.Fatalf("round trip = %+v, want %+v", got, cur)
	}
}

func TestUserCursorRoundTrip(t *testing.T) {
	cur := UserCursor{Username: "amy"}
	enc, err := EncodeCursor(cur)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var got UserCursor
	if err := DecodeCursor(enc, &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got != cur {
		t.Fatalf("round trip = %+v, want %+v", got, cur)
	}
}

func TestDecodeCursorRejectsMalformed(t *testing.T) {
	bad := []string{
		"!!!not-base64!!!", // not base64
		"YWJjZA",           // "abc" — not JSON
	}
	for _, raw := range bad {
		var out URLCursor
		if err := DecodeCursor(raw, &out); err != ErrMalformedCursor {
			t.Errorf("DecodeCursor(%q): err = %v, want ErrMalformedCursor", raw, err)
		}
	}

	// An empty JSON object decodes without error into a zero cursor; rejecting
	// zero-key cursors is the caller's responsibility (e.g. services layer).
	var out URLCursor
	if err := DecodeCursor("e30", &out); err != nil {
		t.Fatalf("DecodeCursor({}): unexpected error %v", err)
	}
	if out != (URLCursor{}) {
		t.Fatalf("DecodeCursor({}) = %+v, want zero cursor", out)
	}
}

func TestURLDataNewerThanOrdering(t *testing.T) {
	t0 := time.Unix(1700000000, 0).UTC()
	u := func(code string, at time.Time) *URLData {
		return &URLData{ShortCode: code, CreatedAt: at}
	}

	cases := []struct {
		a, b *URLData
		want bool
	}{
		{u("a", t0), u("b", t0.Add(time.Second)), false}, // older first in DESC order → a not "newer" than b
		{u("b", t0.Add(time.Second)), u("a", t0), true},
		{u("a", t0), u("b", t0), true}, // same time, code ASC tie-break
		{u("b", t0), u("a", t0), false},
		{u("a", t0), u("a", t0), false},
	}
	for i, c := range cases {
		if got := c.a.NewerThan(c.b); got != c.want {
			t.Errorf("case %d: NewerThan(%s,%s) = %v, want %v", i, c.a.ShortCode, c.b.ShortCode, got, c.want)
		}
	}
}

func TestURLDataAfterKeyset(t *testing.T) {
	t0 := time.Unix(1700000000, 0).UTC()
	u := func(code string, at time.Time) *URLData {
		return &URLData{ShortCode: code, CreatedAt: at}
	}
	cur := URLCursor{CreatedAtUnixNano: t0.Add(time.Second).UnixNano(), ShortCode: "m"}

	cases := []struct {
		item *URLData
		want bool
	}{
		{u("a", t0.Add(2*time.Second)), false}, // newer → not on a later page
		{u("a", t0.Add(time.Second)), false},   // same time, code <= cur code → not later
		{u("m", t0.Add(time.Second)), false},
		{u("n", t0.Add(time.Second)), true}, // same time, code > cur code → later
		{u("a", t0), true},                  // older timestamp → later
	}
	for i, c := range cases {
		if got := c.item.AfterKeyset(cur); got != c.want {
			t.Errorf("case %d: AfterKeyset(%s) = %v, want %v", i, c.item.ShortCode, got, c.want)
		}
	}
}
