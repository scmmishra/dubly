package db

import "database/sql"

func Migrate(db *sql.DB) error {
	_, err := db.Exec(schema)
	return err
}

const schema = `
CREATE TABLE IF NOT EXISTS links (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    slug          TEXT    NOT NULL,
    domain        TEXT    NOT NULL,
    destination   TEXT    NOT NULL,
    title         TEXT    NOT NULL DEFAULT '',
    tags          TEXT    NOT NULL DEFAULT '',
    notes         TEXT    NOT NULL DEFAULT '',
    is_active     INTEGER NOT NULL DEFAULT 1,
    created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(slug, domain)
);

CREATE INDEX IF NOT EXISTS idx_links_domain_slug ON links(domain, slug) WHERE is_active = 1;

CREATE TABLE IF NOT EXISTS clicks (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    link_id         INTEGER NOT NULL,
    clicked_at      DATETIME NOT NULL,
    ip              TEXT,
    user_agent      TEXT,
    referer         TEXT,
    referer_domain  TEXT,
    country         TEXT,
    city            TEXT,
    region          TEXT,
    latitude        REAL,
    longitude       REAL,
    browser         TEXT,
    browser_version TEXT,
    os              TEXT,
    device_type     TEXT,
    FOREIGN KEY (link_id) REFERENCES links(id)
);

CREATE INDEX IF NOT EXISTS idx_clicks_link_id ON clicks(link_id);
CREATE INDEX IF NOT EXISTS idx_clicks_clicked_at ON clicks(clicked_at);
`
