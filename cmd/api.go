package main

// AvailabilityData coming from ILS Connector
type AvailabilityData struct {
	Availability struct {
		ID             string            `json:"title_id"`
		Display        map[string]string `json:"display"`
		Items          []*Item           `json:"items"`
		RequestOptions []RequestOption   `json:"request_options"`
		BoundWith      []BoundWithItem   `json:"bound_with"`
	} `json:"availability"`
}

// Item represents a single item inside availability
type Item struct {
	Barcode         string `json:"barcode"`
	OnShelf         bool   `json:"on_shelf"`
	Unavailable     bool   `json:"unavailable"`
	Notice          string `json:"notice"`
	Library         string `json:"library"`
	LibraryID       string `json:"library_id"`
	CurrentLocation string `json:"current_location"`
	HomeLocationID  string `json:"home_location_id"`
	CallNumber      string `json:"call_number"`
	Volume          string `json:"volume"`
	SCNotes         string `json:"special_collections_location"`
	Map             Map    `json:"map"`
}

// Map contains a URL and label for an item location map
type Map struct {
	ID     string `json:"-"`
	Name   string `json:"name"`
	MapURL string `json:"map,omitempty"`
}

// MapLookup is a lookup table to find a map based on location/callNumber
type MapLookup struct {
	CallNumberRange string
	Location        string
	MapID           string
}

// BoundWithItem are related items bound with this work
type BoundWithItem struct {
	IsParent   bool   `json:"is_parent"`
	TitleID    string `json:"title_id"`
	CallNumber string `json:"call_number"`
	Title      string `json:"title"`
	Author     string `json:"author"`
}

// RequestOption is a category of request that a user can make
type RequestOption struct {
	Type             string       `json:"type"`
	Label            string       `json:"button_label"`
	Description      string       `json:"description"`
	CreateURL        string       `json:"create_url"`
	SignInRequired   bool         `json:"sign_in_required"`
	StreamingReserve bool         `json:"streaming_reserve"`
	ItemOptions      []ItemOption `json:"item_options"`
}

// ItemOption is a selectable item in a RequestOption
type ItemOption struct {
	Label      string `json:"label"`
	Barcode    string `json:"barcode"`
	SCNotes    string `json:"notes"`
	Library    string `json:"library"`
	Location   string `json:"location"`
	LocationID string `json:"location_id"`
	Notice     string `json:"notice"`
}

// SolrResponse container
type SolrResponse struct {
	Response struct {
		Docs     []SolrDocument `json:"docs,omitempty"`
		NumFound int            `json:"numFound,omitempty"`
	} `json:"response,omitempty"`
}

// SolrDocument contains fields for a single solr record (borrowed from the solr pool)
type SolrDocument struct {
	AnonAvailability  []string `json:"anon_availability_a,omitempty"`
	Author            []string `json:"author_a,omitempty"`
	Barcode           []string `json:"barcode_a,omitempty"`
	CallNumber        []string `json:"call_number_a,omitempty"`
	Copy              string   `json:"-"`
	Description       []string `json:"description_a,omitempty"`
	Edition           string   `json:"-"`
	HathiETAS         []string `json:"hathi_etas_f,omitempty"`
	Issue             string   `json:"-"`
	Format            []string `json:"format_a,omitempty"`
	ID                string   `json:"id,omitempty"`
	ISBN              []string `json:"isbn_a,omitempty"`
	ISSN              []string `json:"issn_a,omitempty"`
	Library           []string `json:"library_a,omitempty"`
	Location          []string `json:"location2_a,omitempty"`
	LocalNotes        []string `json:"local_notes_a,omitempty"`
	Medium            []string `json:"medium_a,omitempty"`
	Pool              []string `json:"pool_f,omitempty"`
	PublicationDate   string   `json:"published_date,omitempty"`
	PublishedLocation []string `json:"published_location_a,omitempty"`
	PublisherName     []string `json:"publisher_name_a,omitempty"`
	SCAvailability    string   `json:"sc_availability_large_single,omitempty"`
	Source            []string `json:"source_a,omitempty"`
	Title             []string `json:"title_a,omitempty"`
	URL               []string `json:"url_a,omitempty"`
	Volume            string   `json:"-"`
	WorkTypes         []string `json:"workType_a,omitempty" json:"type_of_record_a,omitempty" json:"medium_a,omitempty"`
}
