package googlebooks

type volumeSearchResponse struct {
	TotalItems int          `json:"totalItems"`
	Items      []volumeItem `json:"items"`
}

type volumeItem struct {
	ID         string     `json:"id"`
	VolumeInfo volumeInfo `json:"volumeInfo"`
}

type volumeInfo struct {
	Title               string            `json:"title"`
	Authors             []string          `json:"authors"`
	Publisher           string            `json:"publisher"`
	PublishedDate       string            `json:"publishedDate"`
	Description         string            `json:"description"`
	PageCount           int               `json:"pageCount"`
	Categories          []string          `json:"categories"`
	AverageRating       float64           `json:"averageRating"`
	RatingsCount        int               `json:"ratingsCount"`
	MaturityRating      string            `json:"maturityRating"`
	ImageLinks          *imageLinks       `json:"imageLinks"`
	IndustryIdentifiers []industryID      `json:"industryIdentifiers"`
	Language            string            `json:"language"`
}

type imageLinks struct {
	SmallThumbnail string `json:"smallThumbnail"`
	Thumbnail      string `json:"thumbnail"`
}

type industryID struct {
	Type       string `json:"type"`       // "ISBN_10" or "ISBN_13"
	Identifier string `json:"identifier"`
}
