package web

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/chatwoot/dubly/internal/models"
)

type AnalyticsData struct {
	PageData
	Link         models.Link
	TotalClicks  int
	TopReferrers []models.ReferrerCount
	TopCountries []models.CountryCount
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
	topReferrers, _ := models.TopReferrersForLink(h.db, id, 5)
	topCountries, _ := models.TopCountriesForLink(h.db, id, 5)

	data := AnalyticsData{
		PageData:     PageData{Flash: getFlash(w, r)},
		Link:         *link,
		TotalClicks:  totalClicks,
		TopReferrers: topReferrers,
		TopCountries: topCountries,
	}

	h.templates.Render(w, "templates/link_analytics.html", data)
}
