package web

import (
	"net/http"

	"github.com/chatwoot/dubly/internal/models"
)

type DashboardData struct {
	PageData
	TotalLinks    int
	ClicksToday   int
	ClicksAllTime int
	TopLinks      []models.LinkWithClicks
	TopReferrers  []models.ReferrerCount
}

func (h *AdminHandler) Dashboard(w http.ResponseWriter, r *http.Request) {
	totalLinks, _ := models.TotalLinkCount(h.db)
	clicksToday, _ := models.ClicksToday(h.db)
	clicksAllTime, _ := models.ClicksAllTime(h.db)
	topLinks, _ := models.TopLinksByClicks(h.db, 5)
	topReferrers, _ := models.TopReferrersGlobal(h.db, 5)

	data := DashboardData{
		PageData:      PageData{Flash: getFlash(w, r)},
		TotalLinks:    totalLinks,
		ClicksToday:   clicksToday,
		ClicksAllTime: clicksAllTime,
		TopLinks:      topLinks,
		TopReferrers:  topReferrers,
	}

	h.templates.Render(w, "templates/dashboard.html", data)
}
