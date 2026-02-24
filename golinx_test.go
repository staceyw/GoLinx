package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Test infrastructure
// ---------------------------------------------------------------------------

func TestMain(m *testing.M) {
	var err error
	db, err = NewSQLiteDB(":memory:")
	if err != nil {
		fmt.Fprintf(os.Stderr, "setup: %v\n", err)
		os.Exit(1)
	}
	*maxResolveDepth = 5
	currentUser = func(r *http.Request) (string, error) {
		return "test@example.com", nil
	}
	code := m.Run()
	db.db.Close()
	os.Exit(code)
}

func resetDB(t *testing.T) {
	t.Helper()
	db.mu.Lock()
	defer db.mu.Unlock()
	if _, err := db.db.Exec("DELETE FROM Cards"); err != nil {
		t.Fatalf("resetDB Cards: %v", err)
	}
	if _, err := db.db.Exec("DELETE FROM Settings"); err != nil {
		t.Fatalf("resetDB Settings: %v", err)
	}
}

func testCards() []*Card {
	return []*Card{
		// Links
		{Type: CardTypeLink, ShortName: "github", DestinationURL: "https://github.com", Description: "GitHub", Owner: "test@example.com"},
		{Type: CardTypeLink, ShortName: "google", DestinationURL: "https://google.com", Description: "Google", Owner: "test@example.com"},
		{Type: CardTypeLink, ShortName: "docs", DestinationURL: "https://docs.google.com/", Description: "Google Docs", Owner: "test@example.com"},
		// Chain: chain-a → chain-b → chain-c (external)
		{Type: CardTypeLink, ShortName: "chain-a", DestinationURL: "/chain-b", Owner: "test@example.com"},
		{Type: CardTypeLink, ShortName: "chain-b", DestinationURL: "/chain-c", Owner: "test@example.com"},
		{Type: CardTypeLink, ShortName: "chain-c", DestinationURL: "https://example.com/final", Owner: "test@example.com"},
		// Template URLs
		{Type: CardTypeLink, ShortName: "search", DestinationURL: "https://www.google.com/search?q={{.Path}}", Description: "Google search", Owner: "test@example.com"},
		{Type: CardTypeLink, ShortName: "myprofile", DestinationURL: "https://corp.example.com/{{.User}}", Owner: "test@example.com"},
		// People
		{Type: CardTypeEmployee, ShortName: "john", FirstName: "John", LastName: "Doe", Title: "Engineer", Email: "john@example.com", Phone: "555-1234", Owner: "test@example.com"},
		{Type: CardTypeCustomer, ShortName: "acme", FirstName: "Acme", LastName: "Corp", Email: "contact@acme.com", Owner: "test@example.com"},
		{Type: CardTypeVendor, ShortName: "vendor1", FirstName: "Vendor", LastName: "One", Email: "v1@vendor.com", Owner: "test@example.com"},
	}
}

func seedTestData(t *testing.T) map[string]int64 {
	t.Helper()
	ids := make(map[string]int64)
	for _, c := range testCards() {
		id, err := db.Save(c)
		if err != nil {
			t.Fatalf("seedTestData(%s): %v", c.ShortName, err)
		}
		ids[c.ShortName] = id
	}
	return ids
}

func doJSON(t *testing.T, mux http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf *bytes.Buffer
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		buf = bytes.NewBuffer(b)
	} else {
		buf = &bytes.Buffer{}
	}
	req := httptest.NewRequest(method, path, buf)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
}

func doRequest(t *testing.T, mux http.Handler, method, url string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, url, nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
}

// ---------------------------------------------------------------------------
// 1. Pure function tests
// ---------------------------------------------------------------------------

func TestExpandLink(t *testing.T) {
	tests := []struct {
		name    string
		long    string
		env     expandEnv
		want    string
		wantErr bool
	}{
		{
			name: "plain URL no path",
			long: "https://github.com",
			env:  expandEnv{Now: time.Now().UTC()},
			want: "https://github.com",
		},
		{
			name: "plain URL with path",
			long: "https://github.com",
			env:  expandEnv{Now: time.Now().UTC(), Path: "anthropics/claude"},
			want: "https://github.com/anthropics/claude",
		},
		{
			name: "trailing slash with path",
			long: "https://docs.google.com/",
			env:  expandEnv{Now: time.Now().UTC(), Path: "extra"},
			want: "https://docs.google.com/extra",
		},
		{
			name: "trailing slash no path",
			long: "https://docs.google.com/",
			env:  expandEnv{Now: time.Now().UTC()},
			want: "https://docs.google.com/",
		},
		{
			name: "template with .Path",
			long: "https://www.google.com/search?q={{.Path}}",
			env:  expandEnv{Now: time.Now().UTC(), Path: "test query"},
			want: "https://www.google.com/search?q=test query",
		},
		{
			name: "template with .User",
			long: "https://corp.example.com/{{.User}}",
			env:  expandEnv{Now: time.Now().UTC(), user: "alice"},
			want: "https://corp.example.com/alice",
		},
		{
			name:    "template with .User no user",
			long:    "https://corp.example.com/{{.User}}",
			env:     expandEnv{Now: time.Now().UTC(), user: ""},
			wantErr: true,
		},
		{
			name: "query merge",
			long: "https://site.com",
			env:  expandEnv{Now: time.Now().UTC(), query: url.Values{"tab": {"repos"}}},
			want: "https://site.com?tab=repos",
		},
		{
			name: "PathEscape func",
			long: "https://host.com/{{PathEscape .Path}}",
			env:  expandEnv{Now: time.Now().UTC(), Path: "a/b"},
			want: "https://host.com/a%2Fb",
		},
		{
			name: "QueryEscape func",
			long: "https://host.com/?q={{QueryEscape .Path}}",
			env:  expandEnv{Now: time.Now().UTC(), Path: "a b"},
			want: "https://host.com/?q=a+b",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := expandLink(tt.long, tt.env)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.String() != tt.want {
				t.Errorf("got %q, want %q", got.String(), tt.want)
			}
		})
	}
}

func TestExtractLocalShortName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"/foo", "foo"},
		{"/foo/bar", ""},
		{"/", ""},
		{"", ""},
		{"https://external.com/single", "single"},
		{"https://external.com/a/b", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			u, _ := url.Parse(tt.input)
			got := extractLocalShortName(u)
			if got != tt.want {
				t.Errorf("extractLocalShortName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestDetectLinkLoop(t *testing.T) {
	t.Run("external URL no loop", func(t *testing.T) {
		resetDB(t)
		seedTestData(t)
		msg := detectLinkLoop("github", "https://external.com")
		if msg != "" {
			t.Errorf("expected no loop, got %q", msg)
		}
	})

	t.Run("direct self-loop", func(t *testing.T) {
		resetDB(t)
		msg := detectLinkLoop("foo", "/foo")
		if msg == "" || !strings.Contains(msg, "loop") {
			t.Errorf("expected loop message, got %q", msg)
		}
	})

	t.Run("two-hop loop", func(t *testing.T) {
		resetDB(t)
		db.Save(&Card{Type: CardTypeLink, ShortName: "loopA", DestinationURL: "/loopB", Owner: "test@example.com"})
		msg := detectLinkLoop("loopB", "/loopA")
		if msg == "" || !strings.Contains(msg, "loop") {
			t.Errorf("expected loop message, got %q", msg)
		}
	})

	t.Run("chain ending external no loop", func(t *testing.T) {
		resetDB(t)
		db.Save(&Card{Type: CardTypeLink, ShortName: "extA", DestinationURL: "/extB", Owner: "test@example.com"})
		db.Save(&Card{Type: CardTypeLink, ShortName: "extB", DestinationURL: "https://example.com", Owner: "test@example.com"})
		msg := detectLinkLoop("newlink", "/extA")
		if msg != "" {
			t.Errorf("expected no loop, got %q", msg)
		}
	})

	t.Run("non-link card breaks chain", func(t *testing.T) {
		resetDB(t)
		db.Save(&Card{Type: CardTypeEmployee, ShortName: "emp1", FirstName: "Test", Owner: "test@example.com"})
		msg := detectLinkLoop("newlink", "/emp1")
		if msg != "" {
			t.Errorf("expected no loop (person card), got %q", msg)
		}
	})
}

// ---------------------------------------------------------------------------
// 2. Database layer tests
// ---------------------------------------------------------------------------

func TestDB_SaveAndLoad(t *testing.T) {
	resetDB(t)

	card := &Card{Type: CardTypeLink, ShortName: "dbtest1", DestinationURL: "https://example.com", Owner: "test@example.com"}
	id, err := db.Save(card)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if id <= 0 {
		t.Fatalf("expected positive ID, got %d", id)
	}

	// LoadByID
	loaded, err := db.LoadByID(id)
	if err != nil {
		t.Fatalf("LoadByID: %v", err)
	}
	if loaded.ShortName != "dbtest1" {
		t.Errorf("ShortName = %q, want %q", loaded.ShortName, "dbtest1")
	}
	if loaded.DestinationURL != "https://example.com" {
		t.Errorf("DestinationURL = %q, want %q", loaded.DestinationURL, "https://example.com")
	}

	// LoadByShortName (case-insensitive)
	loaded2, err := db.LoadByShortName("DBTEST1")
	if err != nil {
		t.Fatalf("LoadByShortName: %v", err)
	}
	if loaded2.ID != id {
		t.Errorf("case-insensitive lookup returned different ID: %d vs %d", loaded2.ID, id)
	}

	// Not found
	_, err = db.LoadByID(999999)
	if err != fs.ErrNotExist {
		t.Errorf("LoadByID(999999) = %v, want fs.ErrNotExist", err)
	}
	_, err = db.LoadByShortName("nonexistent")
	if err != fs.ErrNotExist {
		t.Errorf("LoadByShortName(nonexistent) = %v, want fs.ErrNotExist", err)
	}
}

func TestDB_Update(t *testing.T) {
	resetDB(t)

	id, _ := db.Save(&Card{Type: CardTypeLink, ShortName: "upd1", DestinationURL: "https://old.com", Owner: "test@example.com"})
	err := db.Update(&Card{ID: id, Type: CardTypeLink, ShortName: "upd1", DestinationURL: "https://new.com", Owner: "test@example.com"})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	loaded, _ := db.LoadByID(id)
	if loaded.DestinationURL != "https://new.com" {
		t.Errorf("DestinationURL after update = %q, want %q", loaded.DestinationURL, "https://new.com")
	}

	// Update non-existent
	err = db.Update(&Card{ID: 999999, Type: CardTypeLink, ShortName: "nope"})
	if err != fs.ErrNotExist {
		t.Errorf("Update(999999) = %v, want fs.ErrNotExist", err)
	}
}

func TestDB_Delete(t *testing.T) {
	resetDB(t)

	id, _ := db.Save(&Card{Type: CardTypeLink, ShortName: "del1", DestinationURL: "https://example.com", Owner: "test@example.com"})
	if err := db.Delete(id); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err := db.LoadByID(id)
	if err != fs.ErrNotExist {
		t.Errorf("LoadByID after delete = %v, want fs.ErrNotExist", err)
	}

	// Double delete
	if err := db.Delete(id); err != fs.ErrNotExist {
		t.Errorf("double Delete = %v, want fs.ErrNotExist", err)
	}
}

func TestDB_LoadAll(t *testing.T) {
	resetDB(t)
	seedTestData(t)

	// All cards
	all, err := db.LoadAll("")
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(all) != len(testCards()) {
		t.Errorf("LoadAll() returned %d cards, want %d", len(all), len(testCards()))
	}

	// Filter by type
	links, err := db.LoadAll(CardTypeLink)
	if err != nil {
		t.Fatalf("LoadAll(link): %v", err)
	}
	for _, c := range links {
		if c.Type != CardTypeLink {
			t.Errorf("filter returned non-link card: %s (type %s)", c.ShortName, c.Type)
		}
	}
	if len(links) != 8 { // github, google, docs, chain-a/b/c, search, myprofile
		t.Errorf("LoadAll(link) = %d cards, want 8", len(links))
	}

	employees, err := db.LoadAll(CardTypeEmployee)
	if err != nil {
		t.Fatalf("LoadAll(employee): %v", err)
	}
	if len(employees) != 1 {
		t.Errorf("LoadAll(employee) = %d cards, want 1", len(employees))
	}
}

func TestDB_IncrementClick(t *testing.T) {
	resetDB(t)
	db.Save(&Card{Type: CardTypeLink, ShortName: "clicks1", DestinationURL: "https://example.com", Owner: "test@example.com"})

	if err := db.IncrementClick("clicks1"); err != nil {
		t.Fatalf("IncrementClick: %v", err)
	}
	card, _ := db.LoadByShortName("clicks1")
	if card.ClickCount != 1 {
		t.Errorf("ClickCount = %d, want 1", card.ClickCount)
	}
	if card.LastClicked == 0 {
		t.Error("LastClicked should be non-zero after click")
	}

	// Second click
	db.IncrementClick("clicks1")
	card, _ = db.LoadByShortName("clicks1")
	if card.ClickCount != 2 {
		t.Errorf("ClickCount = %d, want 2", card.ClickCount)
	}
}

func TestDB_CardCount(t *testing.T) {
	resetDB(t)

	count, err := db.CardCount("")
	if err != nil {
		t.Fatalf("CardCount: %v", err)
	}
	if count != 0 {
		t.Errorf("empty DB count = %d, want 0", count)
	}

	seedTestData(t)
	count, _ = db.CardCount("")
	if count != len(testCards()) {
		t.Errorf("total count = %d, want %d", count, len(testCards()))
	}

	linkCount, _ := db.CardCount(CardTypeLink)
	if linkCount != 8 {
		t.Errorf("link count = %d, want 8", linkCount)
	}
}

// ---------------------------------------------------------------------------
// 3. API handler tests
// ---------------------------------------------------------------------------

func TestAPI_CardsList(t *testing.T) {
	mux := serveHandler()

	t.Run("empty database", func(t *testing.T) {
		resetDB(t)
		w := doRequest(t, mux, "GET", "/api/cards")
		if w.Code != 200 {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		var cards []Card
		json.Unmarshal(w.Body.Bytes(), &cards)
		if len(cards) != 0 {
			t.Errorf("expected empty array, got %d cards", len(cards))
		}
	})

	t.Run("all cards", func(t *testing.T) {
		resetDB(t)
		seedTestData(t)
		w := doRequest(t, mux, "GET", "/api/cards")
		if w.Code != 200 {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		var cards []Card
		json.Unmarshal(w.Body.Bytes(), &cards)
		if len(cards) != len(testCards()) {
			t.Errorf("got %d cards, want %d", len(cards), len(testCards()))
		}
	})

	t.Run("filter by type=link", func(t *testing.T) {
		resetDB(t)
		seedTestData(t)
		w := doRequest(t, mux, "GET", "/api/cards?type=link")
		var cards []Card
		json.Unmarshal(w.Body.Bytes(), &cards)
		for _, c := range cards {
			if c.Type != CardTypeLink {
				t.Errorf("got non-link card %s (type %s)", c.ShortName, c.Type)
			}
		}
	})

	t.Run("filter by type=employee", func(t *testing.T) {
		resetDB(t)
		seedTestData(t)
		w := doRequest(t, mux, "GET", "/api/cards?type=employee")
		var cards []Card
		json.Unmarshal(w.Body.Bytes(), &cards)
		if len(cards) != 1 {
			t.Errorf("got %d employees, want 1", len(cards))
		}
	})

	t.Run("invalid type", func(t *testing.T) {
		resetDB(t)
		w := doRequest(t, mux, "GET", "/api/cards?type=bogus")
		if w.Code != 400 {
			t.Errorf("status = %d, want 400", w.Code)
		}
	})
}

func TestAPI_CardsCreate(t *testing.T) {
	mux := serveHandler()

	t.Run("valid link", func(t *testing.T) {
		resetDB(t)
		w := doJSON(t, mux, "POST", "/api/cards", map[string]string{
			"type": "link", "shortName": "newlink", "destinationURL": "https://example.com",
		})
		if w.Code != 201 {
			t.Fatalf("status = %d, want 201; body: %s", w.Code, w.Body.String())
		}
		var card Card
		json.Unmarshal(w.Body.Bytes(), &card)
		if card.ShortName != "newlink" {
			t.Errorf("shortName = %q, want %q", card.ShortName, "newlink")
		}
		if card.Owner != "test@example.com" {
			t.Errorf("owner = %q, want auto-set to test@example.com", card.Owner)
		}
	})

	t.Run("valid employee", func(t *testing.T) {
		resetDB(t)
		w := doJSON(t, mux, "POST", "/api/cards", map[string]string{
			"type": "employee", "shortName": "newemp", "firstName": "Test",
		})
		if w.Code != 201 {
			t.Fatalf("status = %d, want 201; body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("type defaults to link", func(t *testing.T) {
		resetDB(t)
		w := doJSON(t, mux, "POST", "/api/cards", map[string]string{
			"shortName": "deftype", "destinationURL": "https://example.com",
		})
		if w.Code != 201 {
			t.Fatalf("status = %d, want 201", w.Code)
		}
		var card Card
		json.Unmarshal(w.Body.Bytes(), &card)
		if card.Type != CardTypeLink {
			t.Errorf("type = %q, want %q", card.Type, CardTypeLink)
		}
	})

	t.Run("missing shortName", func(t *testing.T) {
		resetDB(t)
		w := doJSON(t, mux, "POST", "/api/cards", map[string]string{
			"type": "link", "destinationURL": "https://example.com",
		})
		if w.Code != 400 {
			t.Errorf("status = %d, want 400", w.Code)
		}
	})

	t.Run("invalid shortName", func(t *testing.T) {
		resetDB(t)
		w := doJSON(t, mux, "POST", "/api/cards", map[string]string{
			"type": "link", "shortName": "has spaces", "destinationURL": "https://example.com",
		})
		if w.Code != 400 {
			t.Errorf("status = %d, want 400", w.Code)
		}
	})

	t.Run("missing destinationURL for link", func(t *testing.T) {
		resetDB(t)
		w := doJSON(t, mux, "POST", "/api/cards", map[string]string{
			"type": "link", "shortName": "nourl",
		})
		if w.Code != 400 {
			t.Errorf("status = %d, want 400", w.Code)
		}
	})

	t.Run("invalid URL scheme", func(t *testing.T) {
		resetDB(t)
		w := doJSON(t, mux, "POST", "/api/cards", map[string]string{
			"type": "link", "shortName": "badurl", "destinationURL": "ftp://bad.com",
		})
		if w.Code != 400 {
			t.Errorf("status = %d, want 400", w.Code)
		}
	})

	t.Run("loop detection", func(t *testing.T) {
		resetDB(t)
		// Create loopX first, then try to create loopY pointing to itself.
		// Create a link, then try to create one pointing to itself.
		doJSON(t, mux, "POST", "/api/cards", map[string]string{
			"type": "link", "shortName": "loopX", "destinationURL": "https://example.com",
		})
		w := doJSON(t, mux, "POST", "/api/cards", map[string]string{
			"type": "link", "shortName": "loopY", "destinationURL": "http://localhost/loopY",
		})
		if w.Code != 400 {
			t.Errorf("status = %d, want 400 for self-loop", w.Code)
		}
		if !strings.Contains(w.Body.String(), "loop") {
			t.Errorf("body should mention loop: %s", w.Body.String())
		}
	})

	t.Run("missing firstName for person", func(t *testing.T) {
		resetDB(t)
		w := doJSON(t, mux, "POST", "/api/cards", map[string]string{
			"type": "employee", "shortName": "nofirst",
		})
		if w.Code != 400 {
			t.Errorf("status = %d, want 400", w.Code)
		}
	})

	t.Run("duplicate shortName", func(t *testing.T) {
		resetDB(t)
		doJSON(t, mux, "POST", "/api/cards", map[string]string{
			"type": "link", "shortName": "dup", "destinationURL": "https://example.com",
		})
		w := doJSON(t, mux, "POST", "/api/cards", map[string]string{
			"type": "link", "shortName": "dup", "destinationURL": "https://other.com",
		})
		if w.Code != 409 {
			t.Errorf("status = %d, want 409", w.Code)
		}
	})
}

func TestAPI_CardColor(t *testing.T) {
	mux := serveHandler()

	t.Run("color round-trip", func(t *testing.T) {
		resetDB(t)
		w := doJSON(t, mux, "POST", "/api/cards", map[string]string{
			"type": "link", "shortName": "colored", "destinationURL": "https://example.com",
			"color": "#ef4444",
		})
		if w.Code != 201 {
			t.Fatalf("create status = %d, want 201; body: %s", w.Code, w.Body.String())
		}
		var card Card
		json.Unmarshal(w.Body.Bytes(), &card)
		if card.Color != "#ef4444" {
			t.Errorf("color = %q, want %q", card.Color, "#ef4444")
		}

		// Update color
		w = doJSON(t, mux, "PUT", fmt.Sprintf("/api/cards/%d", card.ID), map[string]string{
			"type": "link", "shortName": "colored", "destinationURL": "https://example.com",
			"color": "#3b82f6",
		})
		if w.Code != 200 {
			t.Fatalf("update status = %d, want 200; body: %s", w.Code, w.Body.String())
		}
		json.Unmarshal(w.Body.Bytes(), &card)
		if card.Color != "#3b82f6" {
			t.Errorf("color = %q, want %q", card.Color, "#3b82f6")
		}

		// Clear color
		w = doJSON(t, mux, "PUT", fmt.Sprintf("/api/cards/%d", card.ID), map[string]string{
			"type": "link", "shortName": "colored", "destinationURL": "https://example.com",
			"color": "",
		})
		if w.Code != 200 {
			t.Fatalf("clear status = %d, want 200", w.Code)
		}
		json.Unmarshal(w.Body.Bytes(), &card)
		if card.Color != "" {
			t.Errorf("color = %q, want empty", card.Color)
		}
	})
}

func TestAPI_CardsUpdate(t *testing.T) {
	mux := serveHandler()

	t.Run("valid update", func(t *testing.T) {
		resetDB(t)
		ids := seedTestData(t)
		id := ids["github"]
		w := doJSON(t, mux, "PUT", fmt.Sprintf("/api/cards/%d", id), map[string]string{
			"shortName": "github", "destinationURL": "https://github.com/new", "type": "link",
		})
		if w.Code != 200 {
			t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
		}
		var card Card
		json.Unmarshal(w.Body.Bytes(), &card)
		if card.DestinationURL != "https://github.com/new" {
			t.Errorf("destinationURL = %q, want updated value", card.DestinationURL)
		}
	})

	t.Run("non-existent ID", func(t *testing.T) {
		resetDB(t)
		w := doJSON(t, mux, "PUT", "/api/cards/999999", map[string]string{
			"shortName": "x", "destinationURL": "https://x.com", "type": "link",
		})
		if w.Code != 404 {
			t.Errorf("status = %d, want 404", w.Code)
		}
	})

	t.Run("invalid ID", func(t *testing.T) {
		resetDB(t)
		w := doJSON(t, mux, "PUT", "/api/cards/abc", map[string]string{
			"shortName": "x", "type": "link",
		})
		if w.Code != 400 {
			t.Errorf("status = %d, want 400", w.Code)
		}
	})

	t.Run("loop detection on update", func(t *testing.T) {
		resetDB(t)
		ids := seedTestData(t)
		id := ids["github"]
		w := doJSON(t, mux, "PUT", fmt.Sprintf("/api/cards/%d", id), map[string]string{
			"shortName": "github", "destinationURL": "/github", "type": "link",
		})
		if w.Code != 400 {
			t.Errorf("status = %d, want 400 for self-loop", w.Code)
		}
	})
}

func TestAPI_CardsDelete(t *testing.T) {
	mux := serveHandler()

	t.Run("valid delete", func(t *testing.T) {
		resetDB(t)
		ids := seedTestData(t)
		id := ids["google"]
		w := doRequest(t, mux, "DELETE", fmt.Sprintf("/api/cards/%d", id))
		if w.Code != 200 {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		var resp map[string]bool
		json.Unmarshal(w.Body.Bytes(), &resp)
		if !resp["ok"] {
			t.Error("expected ok:true")
		}
	})

	t.Run("not found", func(t *testing.T) {
		resetDB(t)
		w := doRequest(t, mux, "DELETE", "/api/cards/999999")
		if w.Code != 404 {
			t.Errorf("status = %d, want 404", w.Code)
		}
	})

	t.Run("invalid ID", func(t *testing.T) {
		resetDB(t)
		w := doRequest(t, mux, "DELETE", "/api/cards/abc")
		if w.Code != 400 {
			t.Errorf("status = %d, want 400", w.Code)
		}
	})
}

func TestAPI_DBExportImport(t *testing.T) {
	mux := serveHandler()

	t.Run("GET empty", func(t *testing.T) {
		resetDB(t)
		w := doRequest(t, mux, "GET", "/api/db")
		if w.Code != 200 {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		var cards []Card
		json.Unmarshal(w.Body.Bytes(), &cards)
		if len(cards) != 0 {
			t.Errorf("expected empty, got %d", len(cards))
		}
	})

	t.Run("GET with data", func(t *testing.T) {
		resetDB(t)
		seedTestData(t)
		w := doRequest(t, mux, "GET", "/api/db")
		var cards []Card
		json.Unmarshal(w.Body.Bytes(), &cards)
		if len(cards) != len(testCards()) {
			t.Errorf("got %d, want %d", len(cards), len(testCards()))
		}
	})

	t.Run("PUT import new", func(t *testing.T) {
		resetDB(t)
		imports := []map[string]string{
			{"type": "link", "shortName": "imp1", "destinationURL": "https://imp1.com"},
			{"type": "link", "shortName": "imp2", "destinationURL": "https://imp2.com"},
		}
		w := doJSON(t, mux, "PUT", "/api/db", imports)
		if w.Code != 200 {
			t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
		}
		var resp map[string]int
		json.Unmarshal(w.Body.Bytes(), &resp)
		if resp["added"] != 2 {
			t.Errorf("added = %d, want 2", resp["added"])
		}
		if resp["skipped"] != 0 {
			t.Errorf("skipped = %d, want 0", resp["skipped"])
		}
	})

	t.Run("PUT import skips existing", func(t *testing.T) {
		resetDB(t)
		db.Save(&Card{Type: CardTypeLink, ShortName: "existing", DestinationURL: "https://existing.com", Owner: "test@example.com"})
		imports := []map[string]string{
			{"type": "link", "shortName": "existing", "destinationURL": "https://new.com"},
			{"type": "link", "shortName": "brand-new", "destinationURL": "https://brandnew.com"},
		}
		w := doJSON(t, mux, "PUT", "/api/db", imports)
		var resp map[string]int
		json.Unmarshal(w.Body.Bytes(), &resp)
		if resp["added"] != 1 || resp["skipped"] != 1 {
			t.Errorf("got added=%d skipped=%d, want added=1 skipped=1", resp["added"], resp["skipped"])
		}
	})

	t.Run("PUT invalid JSON", func(t *testing.T) {
		resetDB(t)
		req := httptest.NewRequest("PUT", "/api/db", strings.NewReader("not json"))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != 400 {
			t.Errorf("status = %d, want 400", w.Code)
		}
	})
}

func TestAPI_WhoAmI(t *testing.T) {
	mux := serveHandler()
	w := doRequest(t, mux, "GET", "/api/whoami")
	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["login"] != "test@example.com" {
		t.Errorf("login = %q, want %q", resp["login"], "test@example.com")
	}
}

// ---------------------------------------------------------------------------
// 4. Link resolution tests
// ---------------------------------------------------------------------------

func TestResolve_BasicRedirect(t *testing.T) {
	resetDB(t)
	seedTestData(t)
	mux := serveHandler()

	w := doRequest(t, mux, "GET", "/github")
	if w.Code != 302 {
		t.Fatalf("status = %d, want 302", w.Code)
	}
	loc := w.Header().Get("Location")
	if loc != "https://github.com" {
		t.Errorf("Location = %q, want %q", loc, "https://github.com")
	}
}

func TestResolve_PathPassthrough(t *testing.T) {
	resetDB(t)
	seedTestData(t)
	mux := serveHandler()

	w := doRequest(t, mux, "GET", "/github/anthropics/claude")
	if w.Code != 302 {
		t.Fatalf("status = %d, want 302", w.Code)
	}
	loc := w.Header().Get("Location")
	if loc != "https://github.com/anthropics/claude" {
		t.Errorf("Location = %q, want path passthrough", loc)
	}
}

func TestResolve_QueryPassthrough(t *testing.T) {
	resetDB(t)
	seedTestData(t)
	mux := serveHandler()

	w := doRequest(t, mux, "GET", "/github?tab=repos")
	if w.Code != 302 {
		t.Fatalf("status = %d, want 302", w.Code)
	}
	loc := w.Header().Get("Location")
	u, _ := url.Parse(loc)
	if u.Query().Get("tab") != "repos" {
		t.Errorf("query not passed through: %q", loc)
	}
}

func TestResolve_TrailingSlashURL(t *testing.T) {
	resetDB(t)
	seedTestData(t)
	mux := serveHandler()

	w := doRequest(t, mux, "GET", "/docs/extra")
	if w.Code != 302 {
		t.Fatalf("status = %d, want 302", w.Code)
	}
	loc := w.Header().Get("Location")
	if loc != "https://docs.google.com/extra" {
		t.Errorf("Location = %q, want trailing-slash passthrough", loc)
	}
}

func TestResolve_PunctuationTrim(t *testing.T) {
	resetDB(t)
	seedTestData(t)
	mux := serveHandler()

	suffixes := []string{".", ",", ")", "]", "}"}
	for _, s := range suffixes {
		t.Run("trailing_"+s, func(t *testing.T) {
			w := doRequest(t, mux, "GET", "/github"+s)
			if w.Code != 302 {
				t.Errorf("status = %d, want 302 for /github%s", w.Code, s)
			}
			loc := w.Header().Get("Location")
			if !strings.HasPrefix(loc, "https://github.com") {
				t.Errorf("Location = %q, want github redirect", loc)
			}
		})
	}
}

func TestResolve_DetailPage(t *testing.T) {
	resetDB(t)
	seedTestData(t)
	mux := serveHandler()

	t.Run("link detail", func(t *testing.T) {
		w := doRequest(t, mux, "GET", "/github+")
		if w.Code != 200 {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		ct := w.Header().Get("Content-Type")
		if !strings.Contains(ct, "text/html") {
			t.Errorf("Content-Type = %q, want text/html", ct)
		}
		body := w.Body.String()
		if !strings.Contains(body, "github") {
			t.Error("detail page should contain short name")
		}
		if !strings.Contains(body, "https://github.com") {
			t.Error("detail page should contain destination URL")
		}
	})

	t.Run("person detail", func(t *testing.T) {
		w := doRequest(t, mux, "GET", "/john+")
		if w.Code != 200 {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		body := w.Body.String()
		if !strings.Contains(body, "John") {
			t.Error("person detail should contain first name")
		}
	})

	t.Run("not found +", func(t *testing.T) {
		w := doRequest(t, mux, "GET", "/nonexistent+")
		if w.Code != 404 {
			t.Errorf("status = %d, want 404", w.Code)
		}
	})
}

func TestResolve_PersonCard(t *testing.T) {
	resetDB(t)
	seedTestData(t)
	mux := serveHandler()

	tests := []struct {
		path     string
		contains string
	}{
		{"/john", "John"},
		{"/acme", "Acme"},
		{"/vendor1", "Vendor"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			w := doRequest(t, mux, "GET", tt.path)
			if w.Code != 200 {
				t.Fatalf("status = %d, want 200", w.Code)
			}
			ct := w.Header().Get("Content-Type")
			if !strings.Contains(ct, "text/html") {
				t.Errorf("Content-Type = %q, want text/html", ct)
			}
			if !strings.Contains(w.Body.String(), tt.contains) {
				t.Errorf("body should contain %q", tt.contains)
			}
		})
	}
}

func TestResolve_Chain(t *testing.T) {
	resetDB(t)
	seedTestData(t)
	mux := serveHandler()

	// chain-a → chain-b → chain-c → https://example.com/final
	w := doRequest(t, mux, "GET", "/chain-a")
	if w.Code != 302 {
		t.Fatalf("status = %d, want 302", w.Code)
	}
	loc := w.Header().Get("Location")
	if loc != "https://example.com/final" {
		t.Errorf("Location = %q, want https://example.com/final (chain should resolve)", loc)
	}
}

func TestResolve_TemplateURL(t *testing.T) {
	resetDB(t)
	seedTestData(t)
	mux := serveHandler()

	t.Run("template with path", func(t *testing.T) {
		w := doRequest(t, mux, "GET", "/search/hello")
		if w.Code != 302 {
			t.Fatalf("status = %d, want 302", w.Code)
		}
		loc := w.Header().Get("Location")
		if !strings.Contains(loc, "q=hello") {
			t.Errorf("Location = %q, want path in query param", loc)
		}
	})

	t.Run("template with .User", func(t *testing.T) {
		w := doRequest(t, mux, "GET", "/myprofile")
		if w.Code != 302 {
			t.Fatalf("status = %d, want 302", w.Code)
		}
		loc := w.Header().Get("Location")
		if loc != "https://corp.example.com/test@example.com" {
			t.Errorf("Location = %q, want user in URL", loc)
		}
	})
}

func TestResolve_MaxDepth(t *testing.T) {
	resetDB(t)
	// Create a chain: d1 → /d2, d2 → /d3, d3 → /d4, d4 → https://end.com
	db.Save(&Card{Type: CardTypeLink, ShortName: "d1", DestinationURL: "/d2", Owner: "test@example.com"})
	db.Save(&Card{Type: CardTypeLink, ShortName: "d2", DestinationURL: "/d3", Owner: "test@example.com"})
	db.Save(&Card{Type: CardTypeLink, ShortName: "d3", DestinationURL: "/d4", Owner: "test@example.com"})
	db.Save(&Card{Type: CardTypeLink, ShortName: "d4", DestinationURL: "https://end.com", Owner: "test@example.com"})

	old := *maxResolveDepth
	*maxResolveDepth = 1
	t.Cleanup(func() { *maxResolveDepth = old })

	mux := serveHandler()
	w := doRequest(t, mux, "GET", "/d1")
	if w.Code != 302 {
		t.Fatalf("status = %d, want 302", w.Code)
	}
	loc := w.Header().Get("Location")
	// With depth=1, d1 expands to /d2, follows 1 hop to d2→/d3, then stops.
	// The final Location should be /d3 (a local path), NOT https://end.com.
	if loc == "https://end.com" {
		t.Error("maxResolveDepth=1 should prevent reaching end of 3-hop chain")
	}
}

func TestResolve_NotFound(t *testing.T) {
	resetDB(t)
	mux := serveHandler()

	w := doRequest(t, mux, "GET", "/nonexistent")
	if w.Code != 404 {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

// ---------------------------------------------------------------------------
// 5. Static / misc handler tests
// ---------------------------------------------------------------------------

func TestServe_Index(t *testing.T) {
	mux := serveHandler()
	w := doRequest(t, mux, "GET", "/")
	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
}

func TestServe_Favicon(t *testing.T) {
	mux := serveHandler()
	w := doRequest(t, mux, "GET", "/favicon.svg")
	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "image/svg+xml") {
		t.Errorf("Content-Type = %q, want image/svg+xml", ct)
	}
}

func TestServe_Help(t *testing.T) {
	mux := serveHandler()
	w := doRequest(t, mux, "GET", "/.help")
	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
	if !strings.Contains(w.Body.String(), "GoLinx") {
		t.Error("help page should contain GoLinx")
	}
}

func TestServe_Export(t *testing.T) {
	resetDB(t)
	seedTestData(t)
	mux := serveHandler()

	w := doRequest(t, mux, "GET", "/.export")
	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	cd := w.Header().Get("Content-Disposition")
	if !strings.Contains(cd, "attachment") {
		t.Errorf("Content-Disposition = %q, want attachment", cd)
	}
	var cards []Card
	if err := json.Unmarshal(w.Body.Bytes(), &cards); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(cards) != len(testCards()) {
		t.Errorf("exported %d cards, want %d", len(cards), len(testCards()))
	}
}

// ── Listener Parsing ─────────────────────────────────────────────

func TestParseListener(t *testing.T) {
	t.Run("valid URIs", func(t *testing.T) {
		tests := []struct {
			name string
			raw  string
		}{
			{"http empty host", "http://:8080"},
			{"http ipv4 any", "http://0.0.0.0:8080"},
			{"http ipv4 loopback", "http://127.0.0.1:8080"},
			{"http ipv6 loopback", "http://[::1]:8080"},
			{"https with certs", "https://0.0.0.0:443;cert=c.pem;key=k.pem"},
			{"ts+https", "ts+https://:443"},
			{"ts+http", "ts+http://:8080"},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				if _, err := parseListener(tc.raw); err != nil {
					t.Errorf("parseListener(%q) failed: %v", tc.raw, err)
				}
			})
		}
	})

	t.Run("invalid URIs", func(t *testing.T) {
		tests := []struct {
			name string
			raw  string
		}{
			{"reject hostname http", "http://localhost:8080"},
			{"reject hostname ts+https", "ts+https://go:443"},
			{"reject hostname https", "https://myhost:443;cert=c.pem;key=k.pem"},
			{"missing scheme", ":8080"},
			{"unknown scheme", "ftp://:21"},
			{"old tailscale scheme", "tailscale://:443"},
			{"missing port", "http://"},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				if _, err := parseListener(tc.raw); err == nil {
					t.Errorf("parseListener(%q) succeeded, want error", tc.raw)
				}
			})
		}
	})

	t.Run("scheme is preserved", func(t *testing.T) {
		l, err := parseListener("ts+https://:443")
		if err != nil {
			t.Fatalf("parseListener failed: %v", err)
		}
		if l.scheme != "ts+https" {
			t.Errorf("scheme = %q, want ts+https", l.scheme)
		}
	})
}

func TestValidateListeners(t *testing.T) {
	httpL := listener{scheme: "http", port: "8080"}
	tsHTTPS := listener{scheme: "ts+https", port: "443"}
	tsHTTP := listener{scheme: "ts+http", port: "80"}

	t.Run("ts without ts-hostname", func(t *testing.T) {
		old := *tsHostname
		*tsHostname = ""
		defer func() { *tsHostname = old }()
		err := validateListeners([]listener{tsHTTPS})
		if err == nil {
			t.Error("expected error for ts+https without ts-hostname")
		}
	})

	t.Run("ts with ts-hostname", func(t *testing.T) {
		old := *tsHostname
		*tsHostname = "go"
		defer func() { *tsHostname = old }()
		err := validateListeners([]listener{tsHTTPS})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("multiple ts listeners", func(t *testing.T) {
		old := *tsHostname
		*tsHostname = "go"
		defer func() { *tsHostname = old }()
		err := validateListeners([]listener{tsHTTPS, tsHTTP})
		if err != nil {
			t.Errorf("unexpected error for multiple ts listeners: %v", err)
		}
	})

	t.Run("http only", func(t *testing.T) {
		err := validateListeners([]listener{httpL})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("empty list", func(t *testing.T) {
		err := validateListeners(nil)
		if err == nil {
			t.Error("expected error for empty listener list")
		}
	})
}

func TestValidateTSHostname(t *testing.T) {
	valid := []string{"go", "golinx", "my-host", "a", "abc123", "a-b-c"}
	for _, name := range valid {
		t.Run("valid/"+name, func(t *testing.T) {
			if err := validateTSHostname(name); err != nil {
				t.Errorf("validateTSHostname(%q) failed: %v", name, err)
			}
		})
	}

	invalid := []string{"", "  ", "-start", "end-", "has space", "has.dot", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}
	for _, name := range invalid {
		t.Run("invalid/"+name, func(t *testing.T) {
			if err := validateTSHostname(name); err == nil {
				t.Errorf("validateTSHostname(%q) succeeded, want error", name)
			}
		})
	}
}

// ── TCP Ping ─────────────────────────────────────────────────────

func TestValidatePingHost(t *testing.T) {
	valid := []struct {
		name, host string
	}{
		{"hostname", "google.com"},
		{"host:port", "google.com:443"},
		{"ipv4", "8.8.8.8"},
		{"ipv4:port", "8.8.8.8:53"},
		{"private ip", "192.168.1.1"},
		{"loopback", "127.0.0.1"},
		{"subdomain", "foo.bar.example.com"},
		{"single label", "myhost"},
	}
	for _, tc := range valid {
		t.Run("valid/"+tc.name, func(t *testing.T) {
			if err := validatePingHost(tc.host); err != nil {
				t.Errorf("validatePingHost(%q) failed: %v", tc.host, err)
			}
		})
	}

	invalid := []struct {
		name, host string
	}{
		{"empty", ""},
		{"too long", strings.Repeat("a", 254)},
		{"bad chars", "host name!"},
		{"port zero", "google.com:0"},
		{"port too high", "google.com:99999"},
		{"port non-numeric", "google.com:abc"},
	}
	for _, tc := range invalid {
		t.Run("invalid/"+tc.name, func(t *testing.T) {
			if err := validatePingHost(tc.host); err == nil {
				t.Errorf("validatePingHost(%q) succeeded, want error", tc.host)
			}
		})
	}
}

func TestParsePingTarget(t *testing.T) {
	tests := []struct {
		input, wantHost, wantPort string
	}{
		{"google.com", "google.com", "80"},
		{"google.com:443", "google.com", "443"},
		{"8.8.8.8", "8.8.8.8", "80"},
		{"8.8.8.8:53", "8.8.8.8", "53"},
		{"myhost", "myhost", "80"},
		{"myhost:8443", "myhost", "8443"},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			h, p := parsePingTarget(tc.input)
			if h != tc.wantHost || p != tc.wantPort {
				t.Errorf("parsePingTarget(%q) = (%q, %q), want (%q, %q)",
					tc.input, h, p, tc.wantHost, tc.wantPort)
			}
		})
	}
}

func TestServe_PingPage(t *testing.T) {
	mux := serveHandler()

	t.Run("serves HTML page", func(t *testing.T) {
		w := doRequest(t, mux, "GET", "/.ping/google.com")
		if w.Code != 200 {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		ct := w.Header().Get("Content-Type")
		if !strings.Contains(ct, "text/html") {
			t.Errorf("Content-Type = %q, want text/html", ct)
		}
		body := w.Body.String()
		if !strings.Contains(body, "google.com") {
			t.Error("page should contain the host")
		}
		if !strings.Contains(body, "EventSource") {
			t.Error("page should contain EventSource JavaScript")
		}
	})

	t.Run("host with port", func(t *testing.T) {
		w := doRequest(t, mux, "GET", "/.ping/google.com:443")
		if w.Code != 200 {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		if !strings.Contains(w.Body.String(), "google.com:443") {
			t.Error("page should show host:port")
		}
	})

	t.Run("rejects empty host", func(t *testing.T) {
		w := doRequest(t, mux, "GET", "/.ping/")
		// /.ping/ with empty host won't match {host}, falls through to serveRedirect
		if w.Code == 200 {
			t.Error("expected non-200 for empty host ping")
		}
	})

	t.Run("rejects bad hostname", func(t *testing.T) {
		w := doRequest(t, mux, "GET", "/.ping/host%20name!")
		if w.Code != 400 {
			t.Errorf("status = %d, want 400", w.Code)
		}
	})
}

// flushRecorder wraps httptest.ResponseRecorder to implement http.Flusher.
type flushRecorder struct {
	*httptest.ResponseRecorder
}

func (f *flushRecorder) Flush() {}

func TestServe_PingSSE(t *testing.T) {
	mux := serveHandler()

	t.Run("SSE content type and events", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/.ping/localhost:1?stream=1", nil)
		w := &flushRecorder{httptest.NewRecorder()}
		mux.ServeHTTP(w, req)

		ct := w.Header().Get("Content-Type")
		if !strings.Contains(ct, "text/event-stream") {
			t.Errorf("Content-Type = %q, want text/event-stream", ct)
		}
		body := w.Body.String()
		if !strings.Contains(body, "event: status") {
			t.Error("SSE stream should contain status event")
		}
		if !strings.Contains(body, "event: info") {
			t.Error("SSE stream should contain info events")
		}
		if !strings.Contains(body, "event: summary") {
			t.Error("SSE stream should contain summary event")
		}
		if !strings.Contains(body, "event: done") {
			t.Error("SSE stream should contain done event")
		}
	})
}

func TestServe_WhoAmIPage(t *testing.T) {
	mux := serveHandler()

	t.Run("serves HTML page", func(t *testing.T) {
		w := doRequest(t, mux, "GET", "/.whoami")
		if w.Code != 200 {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		ct := w.Header().Get("Content-Type")
		if !strings.Contains(ct, "text/html") {
			t.Errorf("Content-Type = %q, want text/html", ct)
		}
		body := w.Body.String()
		if !strings.Contains(body, "Who Am I") {
			t.Error("page should contain title")
		}
		if !strings.Contains(body, "EventSource") {
			t.Error("page should contain EventSource JavaScript")
		}
	})

	t.Run("SSE local mode fallback", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/.whoami?stream=1", nil)
		w := &flushRecorder{httptest.NewRecorder()}
		mux.ServeHTTP(w, req)

		ct := w.Header().Get("Content-Type")
		if !strings.Contains(ct, "text/event-stream") {
			t.Errorf("Content-Type = %q, want text/event-stream", ct)
		}
		body := w.Body.String()
		if !strings.Contains(body, "event: status") {
			t.Error("SSE stream should contain status event")
		}
		if !strings.Contains(body, "local mode") {
			t.Error("SSE stream should indicate local mode when no Tailscale client")
		}
		if !strings.Contains(body, "event: done") {
			t.Error("SSE stream should contain done event")
		}
	})
}
