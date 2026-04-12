package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/vavallee/bindery/internal/db"
	"github.com/vavallee/bindery/internal/downloader/sabnzbd"
	"github.com/vavallee/bindery/internal/models"
)

type QueueHandler struct {
	downloads *db.DownloadRepo
	clients   *db.DownloadClientRepo
	books     *db.BookRepo
	history   *db.HistoryRepo
}

func NewQueueHandler(downloads *db.DownloadRepo, clients *db.DownloadClientRepo, books *db.BookRepo, history *db.HistoryRepo) *QueueHandler {
	return &QueueHandler{downloads: downloads, clients: clients, books: books, history: history}
}

// QueueItem combines local download record with live SABnzbd status.
type QueueItem struct {
	models.Download
	Percentage string `json:"percentage,omitempty"`
	TimeLeft   string `json:"timeLeft,omitempty"`
	Speed      string `json:"speed,omitempty"`
}

func (h *QueueHandler) List(w http.ResponseWriter, r *http.Request) {
	downloads, err := h.downloads.List(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// Overlay live SABnzbd status
	items := make([]QueueItem, len(downloads))
	for i, d := range downloads {
		items[i] = QueueItem{Download: d}
	}

	client, err := h.clients.GetFirstEnabled(r.Context())
	if err == nil && client != nil {
		sab := sabnzbd.New(client.Host, client.Port, client.APIKey, client.UseSSL)
		queue, err := sab.GetQueue(r.Context())
		if err == nil {
			slotMap := make(map[string]sabnzbd.QueueSlot)
			for _, slot := range queue.Slots {
				slotMap[slot.NzoID] = slot
			}
			for i, item := range items {
				if item.SABnzbdNzoID != nil {
					if slot, ok := slotMap[*item.SABnzbdNzoID]; ok {
						items[i].Percentage = slot.Percentage
						items[i].TimeLeft = slot.TimeLeft
					}
				}
			}
		}
	}

	writeJSON(w, http.StatusOK, items)
}

func (h *QueueHandler) Grab(w http.ResponseWriter, r *http.Request) {
	var req struct {
		GUID      string `json:"guid"`
		Title     string `json:"title"`
		NZBURL    string `json:"nzbUrl"`
		Size      int64  `json:"size"`
		BookID    *int64 `json:"bookId"`
		IndexerID *int64 `json:"indexerId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.GUID == "" || req.NZBURL == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "guid and nzbUrl required"})
		return
	}

	// Check for duplicate
	existing, _ := h.downloads.GetByGUID(r.Context(), req.GUID)
	if existing != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "already grabbed"})
		return
	}

	// Get first enabled download client
	client, err := h.clients.GetFirstEnabled(r.Context())
	if err != nil || client == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no enabled download client configured"})
		return
	}

	// Create download record
	dl := &models.Download{
		GUID:             req.GUID,
		BookID:           req.BookID,
		IndexerID:        req.IndexerID,
		DownloadClientID: &client.ID,
		Title:            req.Title,
		NZBURL:           req.NZBURL,
		Size:             req.Size,
		Status:           models.DownloadStatusQueued,
		Protocol:         "usenet",
	}
	if err := h.downloads.Create(r.Context(), dl); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// Send to SABnzbd
	sab := sabnzbd.New(client.Host, client.Port, client.APIKey, client.UseSSL)
	resp, err := sab.AddURL(r.Context(), req.NZBURL, req.Title, client.Category, 0)
	if err != nil {
		slog.Error("failed to send to sabnzbd", "error", err, "title", req.Title)
		h.downloads.SetError(r.Context(), dl.ID, err.Error())
		h.recordHistory(r.Context(), models.HistoryEventDownloadFailed, req.Title, req.BookID, map[string]string{"message": err.Error()})
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to send to SABnzbd: " + err.Error()})
		return
	}

	// Update with NZO ID
	if len(resp.NzoIDs) > 0 {
		nzoID := resp.NzoIDs[0]
		h.downloads.SetNzoID(r.Context(), dl.ID, nzoID)
		dl.SABnzbdNzoID = &nzoID
	}
	h.downloads.UpdateStatus(r.Context(), dl.ID, models.DownloadStatusDownloading)
	dl.Status = models.DownloadStatusDownloading

	h.recordHistory(r.Context(), models.HistoryEventGrabbed, req.Title, req.BookID, map[string]interface{}{
		"size":      req.Size,
		"indexerId": req.IndexerID,
	})

	slog.Info("download grabbed", "title", req.Title, "nzoId", dl.SABnzbdNzoID)
	writeJSON(w, http.StatusAccepted, dl)
}

// recordHistory is a helper to write a history event, swallowing errors.
func (h *QueueHandler) recordHistory(ctx context.Context, eventType, sourceTitle string, bookID *int64, data interface{}) {
	if h.history == nil {
		return
	}
	dataJSON, _ := json.Marshal(data)
	evt := &models.HistoryEvent{
		BookID:      bookID,
		EventType:   eventType,
		SourceTitle: sourceTitle,
		Data:        string(dataJSON),
	}
	if err := h.history.Create(ctx, evt); err != nil {
		slog.Warn("failed to record history", "error", err)
	}
}

func (h *QueueHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)

	downloads, _ := h.downloads.List(r.Context())
	var target *models.Download
	for _, d := range downloads {
		if d.ID == id {
			target = &d
			break
		}
	}
	if target == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "download not found"})
		return
	}

	// Remove from SABnzbd if it has an NZO ID
	if target.SABnzbdNzoID != nil {
		client, err := h.clients.GetFirstEnabled(r.Context())
		if err == nil && client != nil {
			sab := sabnzbd.New(client.Host, client.Port, client.APIKey, client.UseSSL)
			_ = sab.Delete(r.Context(), *target.SABnzbdNzoID, true)
		}
	}

	// Reset book status back to wanted so it reappears on the Wanted page
	if target.BookID != nil {
		book, _ := h.books.GetByID(r.Context(), *target.BookID)
		if book != nil && (book.Status == models.BookStatusDownloading || book.Status == models.BookStatusDownloaded) {
			book.Status = models.BookStatusWanted
			h.books.Update(r.Context(), book)
		}
	}

	h.downloads.Delete(r.Context(), id)
	w.WriteHeader(http.StatusNoContent)
}
