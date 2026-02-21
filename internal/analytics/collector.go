package analytics

import (
	"database/sql"
	"log"
	"net/url"
	"time"

	"github.com/mssola/useragent"

	"github.com/chatwoot/dubly/internal/geo"
	"github.com/chatwoot/dubly/internal/models"
)

type RawClick struct {
	LinkID    int64
	ClickedAt time.Time
	IP        string
	UserAgent string
	Referer   string
}

type Collector struct {
	ch   chan RawClick
	stop chan struct{}
	db   *sql.DB
	geo  *geo.Reader
	done chan struct{}
}

func NewCollector(db *sql.DB, geoReader *geo.Reader, bufferSize int, flushInterval time.Duration) *Collector {
	c := &Collector{
		ch:   make(chan RawClick, bufferSize),
		stop: make(chan struct{}),
		db:   db,
		geo:  geoReader,
		done: make(chan struct{}),
	}
	go c.run(flushInterval)
	return c
}

// Push sends a click event non-blocking. Drops the event if buffer is full.
func (c *Collector) Push(click RawClick) {
	select {
	case c.ch <- click:
	default:
		// buffer full, drop event
	}
}

// Shutdown flushes remaining events and returns.
func (c *Collector) Shutdown() {
	close(c.stop)
	<-c.done
}

func (c *Collector) run(interval time.Duration) {
	defer close(c.done)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.flush()
		case <-c.stop:
			c.flush()
			return
		}
	}
}

func (c *Collector) flush() {
	var batch []RawClick
	for {
		select {
		case raw := <-c.ch:
			batch = append(batch, raw)
		default:
			goto done
		}
	}
done:
	if len(batch) == 0 {
		return
	}

	clicks := make([]models.Click, 0, len(batch))
	for _, raw := range batch {
		clicks = append(clicks, c.enrich(raw))
	}

	if err := models.BatchInsertClicks(c.db, clicks); err != nil {
		log.Printf("analytics flush error: %v", err)
	} else {
		log.Printf("analytics: flushed %d clicks", len(clicks))
	}
}

func (c *Collector) enrich(raw RawClick) models.Click {
	ua := useragent.New(raw.UserAgent)
	browserName, browserVersion := ua.Browser()

	deviceType := "desktop"
	if ua.Mobile() {
		deviceType = "mobile"
	} else if ua.Bot() {
		deviceType = "bot"
	}

	var refererDomain string
	if raw.Referer != "" {
		if u, err := url.Parse(raw.Referer); err == nil {
			refererDomain = u.Host
		}
	}

	geoResult := c.geo.Lookup(raw.IP)

	return models.Click{
		LinkID:         raw.LinkID,
		ClickedAt:      raw.ClickedAt,
		IP:             raw.IP,
		UserAgent:      raw.UserAgent,
		Referer:        raw.Referer,
		RefererDomain:  refererDomain,
		Country:        geoResult.Country,
		City:           geoResult.City,
		Region:         geoResult.Region,
		Latitude:       geoResult.Latitude,
		Longitude:      geoResult.Longitude,
		Browser:        browserName,
		BrowserVersion: browserVersion,
		OS:             ua.OS(),
		DeviceType:     deviceType,
	}
}
