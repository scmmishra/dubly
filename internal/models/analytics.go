package models

import (
	"database/sql"
	"fmt"
)

type ReferrerCount struct {
	Domain string
	Count  int
}

type CountryCount struct {
	Country string
	Count   int
}

type BrowserCount struct {
	Browser string
	Count   int
}

type DeviceCount struct {
	DeviceType string
	Count      int
}

type LinkWithClicks struct {
	Link       Link
	ClickCount int
}

func ClickCountForLink(db *sql.DB, linkID int64) (int, error) {
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM clicks WHERE link_id = ?`, linkID).Scan(&count)
	return count, err
}

func ClicksTodayForLink(db *sql.DB, linkID int64) (int, error) {
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM clicks WHERE link_id = ? AND date(clicked_at) = date('now')`, linkID).Scan(&count)
	return count, err
}

// ClicksThisWeekForLink returns clicks in the last 7 days for a specific link.
func ClicksThisWeekForLink(db *sql.DB, linkID int64) (int, error) {
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM clicks WHERE link_id = ? AND clicked_at >= datetime('now', '-7 days')`, linkID).Scan(&count)
	return count, err
}

// ClicksPrevWeekForLink returns clicks from 14 to 7 days ago for a specific link.
func ClicksPrevWeekForLink(db *sql.DB, linkID int64) (int, error) {
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM clicks WHERE link_id = ? AND clicked_at >= datetime('now', '-14 days') AND clicked_at < datetime('now', '-7 days')`, linkID).Scan(&count)
	return count, err
}

func ClickCountsForLinks(db *sql.DB, ids []int64) (map[int64]int, error) {
	counts := make(map[int64]int, len(ids))
	if len(ids) == 0 {
		return counts, nil
	}

	// Build placeholders
	placeholders := "?"
	args := make([]any, len(ids))
	args[0] = ids[0]
	for i := 1; i < len(ids); i++ {
		placeholders += ",?"
		args[i] = ids[i]
	}

	query := fmt.Sprintf(`SELECT link_id, COUNT(*) FROM clicks WHERE link_id IN (%s) GROUP BY link_id`, placeholders)
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("click counts: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id int64
		var count int
		if err := rows.Scan(&id, &count); err != nil {
			return nil, fmt.Errorf("scan click count: %w", err)
		}
		counts[id] = count
	}
	return counts, rows.Err()
}

func TopReferrersForLink(db *sql.DB, linkID int64, limit int) ([]ReferrerCount, error) {
	rows, err := db.Query(
		`SELECT referer_domain, COUNT(*) as cnt FROM clicks WHERE link_id = ? AND referer_domain != '' GROUP BY referer_domain ORDER BY cnt DESC LIMIT ?`,
		linkID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("top referrers: %w", err)
	}
	defer rows.Close()

	var results []ReferrerCount
	for rows.Next() {
		var r ReferrerCount
		if err := rows.Scan(&r.Domain, &r.Count); err != nil {
			return nil, fmt.Errorf("scan referrer: %w", err)
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

func TopCountriesForLink(db *sql.DB, linkID int64, limit int) ([]CountryCount, error) {
	rows, err := db.Query(
		`SELECT country, COUNT(*) as cnt FROM clicks WHERE link_id = ? AND country != '' GROUP BY country ORDER BY cnt DESC LIMIT ?`,
		linkID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("top countries: %w", err)
	}
	defer rows.Close()

	var results []CountryCount
	for rows.Next() {
		var c CountryCount
		if err := rows.Scan(&c.Country, &c.Count); err != nil {
			return nil, fmt.Errorf("scan country: %w", err)
		}
		results = append(results, c)
	}
	return results, rows.Err()
}

func TopBrowsersForLink(db *sql.DB, linkID int64, limit int) ([]BrowserCount, error) {
	rows, err := db.Query(
		`SELECT browser, COUNT(*) as cnt FROM clicks WHERE link_id = ? AND browser != '' GROUP BY browser ORDER BY cnt DESC LIMIT ?`,
		linkID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("top browsers: %w", err)
	}
	defer rows.Close()

	var results []BrowserCount
	for rows.Next() {
		var b BrowserCount
		if err := rows.Scan(&b.Browser, &b.Count); err != nil {
			return nil, fmt.Errorf("scan browser: %w", err)
		}
		results = append(results, b)
	}
	return results, rows.Err()
}

func TopDevicesForLink(db *sql.DB, linkID int64, limit int) ([]DeviceCount, error) {
	rows, err := db.Query(
		`SELECT device_type, COUNT(*) as cnt FROM clicks WHERE link_id = ? AND device_type != '' GROUP BY device_type ORDER BY cnt DESC LIMIT ?`,
		linkID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("top devices: %w", err)
	}
	defer rows.Close()

	var results []DeviceCount
	for rows.Next() {
		var d DeviceCount
		if err := rows.Scan(&d.DeviceType, &d.Count); err != nil {
			return nil, fmt.Errorf("scan device: %w", err)
		}
		results = append(results, d)
	}
	return results, rows.Err()
}

func TotalLinkCount(db *sql.DB) (int, error) {
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM links WHERE is_active = 1`).Scan(&count)
	return count, err
}

func ClicksToday(db *sql.DB) (int, error) {
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM clicks WHERE date(clicked_at) = date('now')`).Scan(&count)
	return count, err
}

func ClicksAllTime(db *sql.DB) (int, error) {
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM clicks`).Scan(&count)
	return count, err
}

func TopLinksByClicks(db *sql.DB, limit int) ([]LinkWithClicks, error) {
	rows, err := db.Query(
		`SELECT l.id, l.slug, l.domain, l.destination, l.title, l.tags, l.notes, l.is_active, l.created_at, l.updated_at, COUNT(c.id) as click_count
		FROM links l
		LEFT JOIN clicks c ON c.link_id = l.id
		WHERE l.is_active = 1
		GROUP BY l.id
		ORDER BY click_count DESC
		LIMIT ?`, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("top links: %w", err)
	}
	defer rows.Close()

	var results []LinkWithClicks
	for rows.Next() {
		var lc LinkWithClicks
		var active int
		if err := rows.Scan(
			&lc.Link.ID, &lc.Link.Slug, &lc.Link.Domain, &lc.Link.Destination,
			&lc.Link.Title, &lc.Link.Tags, &lc.Link.Notes, &active,
			&lc.Link.CreatedAt, &lc.Link.UpdatedAt, &lc.ClickCount,
		); err != nil {
			return nil, fmt.Errorf("scan link with clicks: %w", err)
		}
		lc.Link.IsActive = active == 1
		lc.Link.FillShortURL()
		results = append(results, lc)
	}
	return results, rows.Err()
}

func TopBrowsersGlobal(db *sql.DB, limit int) ([]BrowserCount, error) {
	rows, err := db.Query(
		`SELECT browser, COUNT(*) as cnt FROM clicks WHERE browser != '' GROUP BY browser ORDER BY cnt DESC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("global browsers: %w", err)
	}
	defer rows.Close()

	var results []BrowserCount
	for rows.Next() {
		var b BrowserCount
		if err := rows.Scan(&b.Browser, &b.Count); err != nil {
			return nil, fmt.Errorf("scan browser: %w", err)
		}
		results = append(results, b)
	}
	return results, rows.Err()
}

func TopDevicesGlobal(db *sql.DB, limit int) ([]DeviceCount, error) {
	rows, err := db.Query(
		`SELECT device_type, COUNT(*) as cnt FROM clicks WHERE device_type != '' GROUP BY device_type ORDER BY cnt DESC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("global devices: %w", err)
	}
	defer rows.Close()

	var results []DeviceCount
	for rows.Next() {
		var d DeviceCount
		if err := rows.Scan(&d.DeviceType, &d.Count); err != nil {
			return nil, fmt.Errorf("scan device: %w", err)
		}
		results = append(results, d)
	}
	return results, rows.Err()
}

func TopCountriesGlobal(db *sql.DB, limit int) ([]CountryCount, error) {
	rows, err := db.Query(
		`SELECT country, COUNT(*) as cnt FROM clicks WHERE country != '' GROUP BY country ORDER BY cnt DESC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("global countries: %w", err)
	}
	defer rows.Close()

	var results []CountryCount
	for rows.Next() {
		var c CountryCount
		if err := rows.Scan(&c.Country, &c.Count); err != nil {
			return nil, fmt.Errorf("scan country: %w", err)
		}
		results = append(results, c)
	}
	return results, rows.Err()
}

func TopReferrersGlobal(db *sql.DB, limit int) ([]ReferrerCount, error) {
	rows, err := db.Query(
		`SELECT referer_domain, COUNT(*) as cnt FROM clicks WHERE referer_domain != '' GROUP BY referer_domain ORDER BY cnt DESC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("global referrers: %w", err)
	}
	defer rows.Close()

	var results []ReferrerCount
	for rows.Next() {
		var r ReferrerCount
		if err := rows.Scan(&r.Domain, &r.Count); err != nil {
			return nil, fmt.Errorf("scan referrer: %w", err)
		}
		results = append(results, r)
	}
	return results, rows.Err()
}
