package web

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/chatwoot/dubly/internal/models"
)

type AnalyticsData struct {
	PageData
	Link           models.Link
	TotalClicks    int
	ClicksToday    int
	ClicksThisWeek int
	ClicksPrevWeek int
	WeekChange     int  // percentage change, e.g. +25 or -10
	WeekChangeUp   bool // true if this week >= prev week
	TopReferrers   []models.ReferrerCount
	TopCountries   []models.CountryCount
}

func (h *AdminHandler) LinkAnalytics(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	link := &models.Link{ID: id}
	if err := models.GetLinkByID(h.db, link); err != nil {
		http.NotFound(w, r)
		return
	}
	link.FillShortURL()

	totalClicks, _ := models.ClickCountForLink(h.db, id)
	clicksToday, _ := models.ClicksTodayForLink(h.db, id)
	clicksThisWeek, _ := models.ClicksThisWeekForLink(h.db, id)
	clicksPrevWeek, _ := models.ClicksPrevWeekForLink(h.db, id)
	topReferrers, _ := models.TopReferrersForLink(h.db, id, 5)
	topCountries, _ := models.TopCountriesForLink(h.db, id, 5)

	weekChange := 0
	weekChangeUp := true
	if clicksPrevWeek > 0 {
		weekChange = ((clicksThisWeek - clicksPrevWeek) * 100) / clicksPrevWeek
	} else if clicksThisWeek > 0 {
		weekChange = 100
	}
	weekChangeUp = clicksThisWeek >= clicksPrevWeek

	data := AnalyticsData{
		PageData:       h.pageData(w, r),
		Link:           *link,
		TotalClicks:    totalClicks,
		ClicksToday:    clicksToday,
		ClicksThisWeek: clicksThisWeek,
		ClicksPrevWeek: clicksPrevWeek,
		WeekChange:     weekChange,
		WeekChangeUp:   weekChangeUp,
		TopReferrers:   topReferrers,
		TopCountries:   topCountries,
	}

	h.templates.Render(w, "templates/link_analytics.html", data)
}
