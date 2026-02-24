package main

import (
	"database/sql"
	_ "embed"
	"errors"
	"io/fs"
	"sync"
	"time"
)

//go:embed schema.sql
var sqlSchema string

// Card type constants.
const (
	CardTypeLink     = "link"
	CardTypeEmployee = "employee"
	CardTypeCustomer = "customer"
	CardTypeVendor   = "vendor"
)

// Card represents any item in GoLinx: a link, employee, customer, or vendor.
type Card struct {
	ID             int64  `json:"id"`
	Type           string `json:"type"`
	ShortName      string `json:"shortName"`
	DestinationURL string `json:"destinationURL"`
	Description    string `json:"description"`
	Owner          string `json:"owner"`
	LastClicked    int64  `json:"lastClicked"`
	ClickCount     int64  `json:"clickCount"`
	FirstName    string `json:"firstName"`
	LastName     string `json:"lastName"`
	Title        string `json:"title"`
	Email        string `json:"email"`
	Phone        string `json:"phone"`
	WebLink      string `json:"webLink"`
	CalLink      string `json:"calLink"`
	XLink        string `json:"xLink"`
	LinkedInLink string `json:"linkedInLink"`
	AvatarMime     string `json:"avatarMime,omitempty"`
	Color          string `json:"color"`
	DateCreated    int64  `json:"dateCreated"`
}

// IsPersonType returns true for card types that use the person form and profile page.
func (c *Card) IsPersonType() bool {
	return c.Type == CardTypeEmployee || c.Type == CardTypeCustomer || c.Type == CardTypeVendor
}

const cardColumns = `ID, Type, ShortName, DestinationURL, Description, Owner, LastClicked, ClickCount, FirstName, LastName, Title, Email, Phone, WebLink, CalLink, XLink, LinkedInLink, AvatarMime, Color, DateCreated`

func scanCard(scanner interface{ Scan(dest ...any) error }) (*Card, error) {
	c := new(Card)
	err := scanner.Scan(&c.ID, &c.Type, &c.ShortName, &c.DestinationURL,
		&c.Description, &c.Owner, &c.LastClicked, &c.ClickCount,
		&c.FirstName, &c.LastName, &c.Title, &c.Email, &c.Phone,
		&c.WebLink, &c.CalLink, &c.XLink, &c.LinkedInLink, &c.AvatarMime, &c.Color, &c.DateCreated)
	return c, err
}

// SQLiteDB wraps the database connection with a mutex for safe concurrent access.
type SQLiteDB struct {
	db *sql.DB
	mu sync.RWMutex
}

// NewSQLiteDB opens a SQLite database and initializes the schema.
func NewSQLiteDB(f string) (*SQLiteDB, error) {
	db, err := sql.Open("sqlite", f)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, err
	}
	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA foreign_keys=ON",
	} {
		if _, err := db.Exec(pragma); err != nil {
			return nil, err
		}
	}
	if _, err = db.Exec(sqlSchema); err != nil {
		return nil, err
	}
	// Migrate: add columns that may not exist in older databases.
	for _, col := range []string{
		"ALTER TABLE Cards ADD COLUMN Color TEXT NOT NULL DEFAULT ''",
	} {
		db.Exec(col) // ignore "duplicate column" errors
	}
	return &SQLiteDB{db: db}, nil
}

// LoadAll returns all cards, optionally filtered by type, ordered by ShortName.
func (s *SQLiteDB) LoadAll(filterType string) ([]*Card, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := "SELECT " + cardColumns + " FROM Cards"
	var args []any
	if filterType != "" {
		query += " WHERE Type = ?"
		args = append(args, filterType)
	}
	query += " ORDER BY LOWER(ShortName)"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cards []*Card
	for rows.Next() {
		c, err := scanCard(rows)
		if err != nil {
			return nil, err
		}
		cards = append(cards, c)
	}
	return cards, rows.Err()
}

// LoadByID returns a single card by ID.
func (s *SQLiteDB) LoadByID(id int64) (*Card, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	c, err := scanCard(s.db.QueryRow("SELECT "+cardColumns+" FROM Cards WHERE ID = ?", id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fs.ErrNotExist
		}
		return nil, err
	}
	return c, nil
}

// LoadByShortName returns a single card by short name (case-insensitive).
func (s *SQLiteDB) LoadByShortName(name string) (*Card, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	c, err := scanCard(s.db.QueryRow("SELECT "+cardColumns+" FROM Cards WHERE LOWER(ShortName) = LOWER(?)", name))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fs.ErrNotExist
		}
		return nil, err
	}
	return c, nil
}

// Save inserts a new card and returns its ID.
func (s *SQLiteDB) Save(card *Card) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.insertCard(card)
}

func (s *SQLiteDB) insertCard(card *Card) (int64, error) {
	if card.Type == "" {
		card.Type = CardTypeLink
	}
	now := time.Now().Unix()
	result, err := s.db.Exec(
		`INSERT INTO Cards (Type, ShortName, DestinationURL, Description, Owner, FirstName, LastName, Title, Email, Phone, WebLink, CalLink, XLink, LinkedInLink, Color, DateCreated) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		card.Type, card.ShortName, card.DestinationURL, card.Description, card.Owner,
		card.FirstName, card.LastName, card.Title, card.Email, card.Phone,
		card.WebLink, card.CalLink, card.XLink, card.LinkedInLink, card.Color, now,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// Update modifies an existing card by ID.
func (s *SQLiteDB) Update(card *Card) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	result, err := s.db.Exec(
		`UPDATE Cards SET Type=?, ShortName=?, DestinationURL=?, Description=?, Owner=?, FirstName=?, LastName=?, Title=?, Email=?, Phone=?, WebLink=?, CalLink=?, XLink=?, LinkedInLink=?, Color=? WHERE ID=?`,
		card.Type, card.ShortName, card.DestinationURL, card.Description, card.Owner,
		card.FirstName, card.LastName, card.Title, card.Email, card.Phone,
		card.WebLink, card.CalLink, card.XLink, card.LinkedInLink, card.Color, card.ID,
	)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fs.ErrNotExist
	}
	return nil
}

// Delete removes a card by ID.
func (s *SQLiteDB) Delete(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	result, err := s.db.Exec("DELETE FROM Cards WHERE ID = ?", id)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fs.ErrNotExist
	}
	return nil
}

// IncrementClick atomically increments click count and updates LastClicked.
func (s *SQLiteDB) IncrementClick(shortName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().Unix()
	_, err := s.db.Exec("UPDATE Cards SET ClickCount = ClickCount + 1, LastClicked = ? WHERE LOWER(ShortName) = LOWER(?)", now, shortName)
	return err
}

// GetSetting retrieves a setting value.
func (s *SQLiteDB) GetSetting(username, key string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var value string
	err := s.db.QueryRow("SELECT value FROM Settings WHERE username = ? AND key = ?", username, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return value, err
}

// PutSetting saves a setting value.
func (s *SQLiteDB) PutSetting(username, key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec("INSERT OR REPLACE INTO Settings (username, key, value) VALUES (?, ?, ?)", username, key, value)
	return err
}

// CardCount returns the total number of cards, optionally filtered by type.
func (s *SQLiteDB) CardCount(filterType string) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := "SELECT COUNT(*) FROM Cards"
	var args []any
	if filterType != "" {
		query += " WHERE Type = ?"
		args = append(args, filterType)
	}
	var count int
	err := s.db.QueryRow(query, args...).Scan(&count)
	return count, err
}

// SaveAvatar updates the avatar for a card.
func (s *SQLiteDB) SaveAvatar(id int64, data []byte, mime string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	result, err := s.db.Exec("UPDATE Cards SET AvatarData = ?, AvatarMime = ? WHERE ID = ?", data, mime, id)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fs.ErrNotExist
	}
	return nil
}

// LoadAvatar returns the avatar data and MIME type for a card.
func (s *SQLiteDB) LoadAvatar(id int64) ([]byte, string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var data []byte
	var mime string
	err := s.db.QueryRow("SELECT AvatarData, AvatarMime FROM Cards WHERE ID = ?", id).Scan(&data, &mime)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, "", fs.ErrNotExist
		}
		return nil, "", err
	}
	return data, mime, nil
}
