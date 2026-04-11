package sabnzbd

// AddURLResponse is returned when adding an NZB by URL.
type AddURLResponse struct {
	Status bool     `json:"status"`
	NzoIDs []string `json:"nzo_ids"`
}

// QueueResponse is the SABnzbd queue status.
type QueueResponse struct {
	Queue QueueData `json:"queue"`
}

type QueueData struct {
	Status   string      `json:"status"`
	Speed    string      `json:"speed"`
	SizeLeft string      `json:"sizeleft"`
	TimeLeft string      `json:"timeleft"`
	Paused   bool        `json:"paused"`
	Slots    []QueueSlot `json:"slots"`
}

type QueueSlot struct {
	NzoID      string `json:"nzo_id"`
	Filename   string `json:"filename"`
	Status     string `json:"status"`
	Category   string `json:"cat"`
	MB         string `json:"mb"`
	MBLeft     string `json:"mbleft"`
	Percentage string `json:"percentage"`
	Priority   string `json:"priority"`
	TimeLeft   string `json:"timeleft"`
}

// HistoryResponse is the SABnzbd history.
type HistoryResponse struct {
	History HistoryData `json:"history"`
}

type HistoryData struct {
	TotalSize string        `json:"total_size"`
	Slots     []HistorySlot `json:"slots"`
}

type HistorySlot struct {
	NzoID       string `json:"nzo_id"`
	Name        string `json:"name"`
	Status      string `json:"status"`
	Category    string `json:"category"`
	Size        string `json:"size"`
	Path        string `json:"storage"`
	FailMessage string `json:"fail_message"`
	Completed   int64  `json:"completed"`
	URL         string `json:"url"`
}

// CategoriesResponse lists configured categories.
type CategoriesResponse struct {
	Categories []string `json:"categories"`
}

// SimpleResponse is a generic OK/error response.
type SimpleResponse struct {
	Status bool   `json:"status"`
	Error  string `json:"error"`
}
