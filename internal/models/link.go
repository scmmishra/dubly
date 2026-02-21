package models

import (
	"database/sql"
	"fmt"
	"time"
)

type Link struct {
	ID          int64     `json:"id"`
	Slug        string    `json:"slug"`
	Domain      string    `json:"domain"`
	ShortURL    string    `json:"short_url"`
	Destination string    `json:"destination"`
	Title       string    `json:"title"`
	Tags        string    `json:"tags"`
	Notes       string    `json:"notes"`
	IsActive    bool      `json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (l *Link) FillShortURL() {
	l.ShortURL = "https://" + l.Domain + "/" + l.Slug
}

func CreateLink(db *sql.DB, l *Link) error {
	res, err := db.Exec(
		`INSERT INTO links (slug, domain, destination, title, tags, notes) VALUES (?, ?, ?, ?, ?, ?)`,
		l.Slug, l.Domain, l.Destination, l.Title, l.Tags, l.Notes,
	)
	if err != nil {
		return fmt.Errorf("insert link: %w", err)
	}
	id, _ := res.LastInsertId()
	l.ID = id

	// Re-read to get timestamps
	return GetLinkByID(db, l)
}

func GetLinkByID(db *sql.DB, l *Link) error {
	row := db.QueryRow(`SELECT id, slug, domain, destination, title, tags, notes, is_active, created_at, updated_at FROM links WHERE id = ?`, l.ID)
	return scanLink(row, l)
}

func GetLinkBySlugAndDomain(db *sql.DB, slug, domain string) (*Link, error) {
	l := &Link{}
	row := db.QueryRow(
		`SELECT id, slug, domain, destination, title, tags, notes, is_active, created_at, updated_at FROM links WHERE domain = ? AND slug = ?`,
		domain, slug,
	)
	if err := scanLink(row, l); err != nil {
		return nil, err
	}
	l.FillShortURL()
	return l, nil
}

func ListLinks(db *sql.DB, limit, offset int, search string) ([]Link, int, error) {
	var args []any
	where := "1=1"
	if search != "" {
		where = "(slug LIKE ? OR destination LIKE ? OR title LIKE ? OR tags LIKE ?)"
		s := "%" + search + "%"
		args = append(args, s, s, s, s)
	}

	var total int
	countQuery := "SELECT COUNT(*) FROM links WHERE " + where
	if err := db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count links: %w", err)
	}

	query := "SELECT id, slug, domain, destination, title, tags, notes, is_active, created_at, updated_at FROM links WHERE " + where + " ORDER BY created_at DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list links: %w", err)
	}
	defer rows.Close()

	var links []Link
	for rows.Next() {
		var l Link
		var active int
		if err := rows.Scan(&l.ID, &l.Slug, &l.Domain, &l.Destination, &l.Title, &l.Tags, &l.Notes, &active, &l.CreatedAt, &l.UpdatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan link: %w", err)
		}
		l.IsActive = active == 1
		l.FillShortURL()
		links = append(links, l)
	}
	return links, total, rows.Err()
}

func UpdateLink(db *sql.DB, l *Link) error {
	_, err := db.Exec(
		`UPDATE links SET slug = ?, domain = ?, destination = ?, title = ?, tags = ?, notes = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		l.Slug, l.Domain, l.Destination, l.Title, l.Tags, l.Notes, l.ID,
	)
	if err != nil {
		return fmt.Errorf("update link: %w", err)
	}
	return GetLinkByID(db, l)
}

func SoftDeleteLink(db *sql.DB, id int64) error {
	res, err := db.Exec(`UPDATE links SET is_active = 0, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("soft delete link: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func SlugExists(db *sql.DB, slug, domain string) (bool, error) {
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM links WHERE slug = ? AND domain = ?`, slug, domain).Scan(&count)
	return count > 0, err
}

func scanLink(row *sql.Row, l *Link) error {
	var active int
	if err := row.Scan(&l.ID, &l.Slug, &l.Domain, &l.Destination, &l.Title, &l.Tags, &l.Notes, &active, &l.CreatedAt, &l.UpdatedAt); err != nil {
		return err
	}
	l.IsActive = active == 1
	l.FillShortURL()
	return nil
}
