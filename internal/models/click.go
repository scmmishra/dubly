package models

import (
	"database/sql"
	"fmt"
	"time"
)

type Click struct {
	ID             int64
	LinkID         int64
	ClickedAt      time.Time
	IP             string
	UserAgent      string
	Referer        string
	RefererDomain  string
	Country        string
	City           string
	Region         string
	Latitude       float64
	Longitude      float64
	Browser        string
	BrowserVersion string
	OS             string
	DeviceType     string
}

func BatchInsertClicks(db *sql.DB, clicks []Click) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT INTO clicks (link_id, clicked_at, ip, user_agent, referer, referer_domain, country, city, region, latitude, longitude, browser, browser_version, os, device_type) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare: %w", err)
	}
	defer stmt.Close()

	for _, c := range clicks {
		_, err := stmt.Exec(
			c.LinkID, c.ClickedAt, c.IP, c.UserAgent, c.Referer, c.RefererDomain,
			c.Country, c.City, c.Region, c.Latitude, c.Longitude,
			c.Browser, c.BrowserVersion, c.OS, c.DeviceType,
		)
		if err != nil {
			return fmt.Errorf("insert click: %w", err)
		}
	}

	return tx.Commit()
}
