package web

import (
	"bytes"
	"io"
	"net/http"
	"regexp"
	"strconv"

	"github.com/go-chi/chi/v5"
	qrcode "github.com/yeqown/go-qrcode/v2"
	"github.com/yeqown/go-qrcode/writer/standard"

	"github.com/scmmishra/dubly/internal/models"
)

var hexColorRe = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func isValidHex(s string) bool {
	return hexColorRe.MatchString(s)
}

type nopCloser struct{ io.Writer }

func (nopCloser) Close() error { return nil }

func (h *AdminHandler) LinkQRCode(w http.ResponseWriter, r *http.Request) {
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

	// Parse query params with defaults
	shape := r.URL.Query().Get("shape") // square|circle
	fg := r.URL.Query().Get("fg")       // hex color
	dl := r.URL.Query().Get("dl")       // 0|1

	// Build image options — always transparent background
	opts := []standard.ImageOption{
		standard.WithBuiltinImageEncoder(standard.PNG_FORMAT),
		standard.WithQRWidth(10),
		standard.WithBorderWidth(20),
		standard.WithBgTransparent(),
	}

	if shape == "circle" {
		opts = append(opts, standard.WithCircleShape())
	}

	if isValidHex(fg) {
		opts = append(opts, standard.WithFgColorRGBHex(fg))
	}



	qrc, err := qrcode.New(link.ShortURL)
	if err != nil {
		http.Error(w, "failed to generate qr code", http.StatusInternalServerError)
		return
	}

	var buf bytes.Buffer
	writer := standard.NewWithWriter(nopCloser{&buf}, opts...)
	if err := qrc.Save(writer); err != nil {
		http.Error(w, "failed to render qr code", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	if dl == "1" {
		w.Header().Set("Content-Disposition", "attachment; filename=\""+link.Slug+"-qr.png\"")
	}
	w.Write(buf.Bytes())
}
