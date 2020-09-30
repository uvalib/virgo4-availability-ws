package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"reflect"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/go-querystring/query"
)

// getAvailability uses ILS Connector V4 API /availability to get details for a Document
func (svc *ServiceContext) getAvailability(c *gin.Context) {
	titleID := c.Param("id")
	log.Printf("Getting availability for %s with ILS Connector...", titleID)

	availabilityURL := fmt.Sprintf("%s/v4/availability/%s", svc.ILSAPI, titleID)
	bodyBytes, ilsErr := svc.ILSConnectorGet(availabilityURL, c.GetString("jwt"), svc.HTTPClient)
	if ilsErr != nil && ilsErr.StatusCode != 404 {
		log.Printf("ERROR: ILS Connector failure: %+v", ilsErr)
		c.String(ilsErr.StatusCode, "There was a problem retrieving availability. Please try again later.")
		return
	}

	if ilsErr != nil && ilsErr.StatusCode == 503 {
		log.Printf("ERROR: Sirsi is offline")
		c.String(ilsErr.StatusCode, "Availability information is currently unavailable. Please try again later.")
		return
	}

	// Convert from json
	var availResp AvailabilityData
	if err := json.Unmarshal(bodyBytes, &availResp); err != nil {
		// Non-Sirsi Item may be found in other places and have availability
		availResp = AvailabilityData{}
	}

	// Create a display mapping from item field to label. Localize at some point. Maybe.
	availResp.Availability.Display = make(map[string]string)
	availResp.Availability.Display["library"] = "Library"
	availResp.Availability.Display["current_location"] = "Current Location"
	availResp.Availability.Display["call_number"] = "Call Number"
	availResp.Availability.Display["barcode"] = "Barcode"

	solrDoc := svc.getSolrDoc(titleID)

	v4Claims, _ := getJWTClaims(c)
	if v4Claims.HomeLibrary == "HEALTHSCI" {
		svc.updateHSLScanOptions(titleID, &solrDoc, &availResp)
	}
	if v4Claims.CanPlaceReserve {
		svc.addStreamingVideoReserve(titleID, &solrDoc, &availResp)
	}

	svc.appendAeonRequestOptions(titleID, &solrDoc, &availResp)
	svc.removeETASRequestOptions(titleID, &solrDoc, &availResp)
	svc.addMapInfo(availResp.Availability.Items)

	c.JSON(http.StatusOK, availResp)
}

func (svc *ServiceContext) getSolrDoc(id string) SolrDocument {
	fields := solrFieldList()
	solrPath := fmt.Sprintf(`select?fl=%s,&q=id%%3A%s`, fields, id)

	respBytes, solrErr := svc.SolrGet(solrPath)
	if solrErr != nil {
		log.Printf("ERROR: Solr request for Aeon info failed: %s", solrErr.Message)
	}
	var SolrResp SolrResponse
	if err := json.Unmarshal(respBytes, &SolrResp); err != nil {
		log.Printf("ERROR: Unable to parse solr response: %s.", err.Error())
	}
	if SolrResp.Response.NumFound != 1 {
		log.Printf("ERROR: Availability - More than one record found for the cat key: %s", id)
	}
	SolrDoc := SolrResp.Response.Docs[0]
	return SolrDoc
}

func (svc *ServiceContext) updateHSLScanOptions(id string, solrDoc *SolrDocument, result *AvailabilityData) {
	log.Printf("Updating scan options for HSL user")
	// remove existing scan
	for i, opt := range result.Availability.RequestOptions {
		if opt.Type == "scan" {
			result.Availability.RequestOptions = append(
				result.Availability.RequestOptions[:i],
				result.Availability.RequestOptions[i+1:]...)
			break
		}
	}

	hsScan := RequestOption{
		Type:           "directLink",
		Label:          "Request a scan",
		SignInRequired: false,
		Description:    "Select a portion of this item to be scanned.",
		CreateURL:      openURLQuery(svc.HSILLiadURL, solrDoc),
		ItemOptions:    make([]ItemOption, 0),
	}
	result.Availability.RequestOptions = append(result.Availability.RequestOptions, hsScan)
}

func openURLQuery(baseURL string, doc *SolrDocument) string {
	var req struct {
		Action  string `url:"Action"`
		Form    string `url:"Form"`
		ISSN    string `url:"issn,omitempty"`
		Title   string `url:"loantitle"`
		Author  string `url:"loanauthor,omitempty"`
		Edition string `url:"loanedition,omitempty"`
		Volume  string `url:"photojournalvolume,omitempty"`
		Issue   string `url:"photojournalissue,omitempty"`
		Date    string `url:"loandate,omitempty"`
	}
	req.Action = "10"
	req.Form = "21"
	req.ISSN = strings.Join(doc.ISSN, ", ")
	req.Title = strings.Join(doc.Title, "; ")
	req.Author = strings.Join(doc.Author, "; ")
	req.Edition = doc.Edition
	req.Volume = doc.Volume
	req.Issue = doc.Issue
	req.Date = doc.PublicationDate
	query, err := query.Values(req)
	if err != nil {
		log.Printf("ERROR: couldn't generate OpenURL: %s", err.Error())
	}

	return fmt.Sprintf("%s/illiad.dll?%s", baseURL, query.Encode())
}

// Adds option for course reserves video request for streaming video items
// This could be Sirsi "Internet materials", Avalon, Swank, etc.
func (svc *ServiceContext) addStreamingVideoReserve(id string, solrDoc *SolrDocument, result *AvailabilityData) {

	if (solrDoc.Pool[0] == "video" && contains(solrDoc.Location, "Internet materials")) ||
		contains(solrDoc.Source, "Avalon") {

		log.Printf("Adding streaming video reserve option")
		option := RequestOption{
			Type:             "videoReserve",
			Label:            "Video reserve request",
			SignInRequired:   true,
			Description:      "Request a video reserve for streaming",
			StreamingReserve: true,
			ItemOptions:      []ItemOption{},
		}
		result.Availability.RequestOptions = append(result.Availability.RequestOptions, option)
	}

	return
}

// Appends Aeon request to availability response
func (svc *ServiceContext) appendAeonRequestOptions(id string, solrDoc *SolrDocument, result *AvailabilityData) {

	processSCAvailabilityStored(result, solrDoc)

	if !(contains(solrDoc.Library, "Special Collections")) {
		return
	}

	aeonOption := RequestOption{
		Type:           "aeon",
		Label:          "Request this in Special Collections",
		SignInRequired: false,
		Description:    "",
		CreateURL:      createAeonURL(solrDoc),
		ItemOptions:    createAeonItemOptions(result, solrDoc),
	}
	result.Availability.RequestOptions = append(result.Availability.RequestOptions, aeonOption)
}

// processSCAvailabilityStored adds items stored in sc_availability_stored solr field to availability
func processSCAvailabilityStored(result *AvailabilityData, doc *SolrDocument) {
	// If this item has Stored SC data (ArchiveSpace)
	if doc.SCAvailability == "" {
		return
	}

	// Complete required availability fields
	result.Availability.ID = doc.ID

	var scItems []*Item
	if err := json.Unmarshal([]byte(doc.SCAvailability), &scItems); err != nil {
		log.Printf("Error parsing sc_availability_large_single: %+v", err)
	}

	for _, item := range scItems {
		result.Availability.Items = append(result.Availability.Items, item)
	}
	return
}

// Creates Aeon ItemOptions based on availability data
func createAeonItemOptions(result *AvailabilityData, doc *SolrDocument) []ItemOption {

	// Sirsi Item Options
	options := []ItemOption{}
	for _, item := range result.Availability.Items {
		if item.LibraryID == "SPEC-COLL" || doc.SCAvailability != "" {
			notes := ""
			if len(item.SCNotes) > 0 {
				notes = item.SCNotes
			} else if len(doc.LocalNotes) > 0 {
				// drop name
				prefix1 := regexp.MustCompile(`^\s*SPECIAL\s+COLLECTIONS:\s+`)
				//shorten SC name
				prefix2 := regexp.MustCompile(`^\s*Harrison Small Special Collections,`)

				for _, note := range doc.LocalNotes {
					note = prefix1.ReplaceAllString(note, "")
					note = prefix2.ReplaceAllString(note, "H. Small,")
					// trim
					notes += (strings.TrimSpace(note) + ";\n")
				}
				// truncate
				if len(notes) > 999 {
					notes = notes[:999]
				}

			} else {
				notes = "(no location notes)"
			}

			scItem := ItemOption{
				Barcode:  item.Barcode,
				Label:    item.CallNumber,
				Location: item.HomeLocationID,
				Library:  item.Library,
				SCNotes:  notes,
				Notice:   item.Notice,
			}
			options = append(options, scItem)
		}
	}

	return options
}

// Create OpenUrl for Aeon
func createAeonURL(doc *SolrDocument) string {

	type aeonRequest struct {
		Action      int    `url:"Action"`
		Form        int    `url:"Form"`
		Value       string `url:"Value"` // either GenericRequestManuscript or GenericRequestMonograph
		DocID       string `url:"ReferenceNumber"`
		Title       string `url:"ItemTitle" default:"(NONE)"`
		Author      string `url:"ItemAuthor"`
		Date        string `url:"ItemDate"`
		ISxN        string `url:"ItemISxN"`
		CallNumber  string `url:"CallNumber" default:"(NONE)"`
		Barcode     string `url:"ItemNumber"`
		Place       string `url:"ItemPlace"`
		Publisher   string `url:"ItemPublisher"`
		Edition     string `url:"ItemEdition"`
		Issue       string `url:"ItemIssuesue"`
		Volume      string `url:"ItemVolume"` // unless manuscript
		Copy        string `url:"ItemInfo2"`
		Location    string `url:"Location"`
		Description string `url:"ItemInfo1"`
		Notes       string `url:"Notes"`
		Tags        string `url:"ResearcherTags,omitempty"`
		UserNote    string `url:"SpecialRequest"`
	}

	// Decide monograph or manuscript
	formValue := "GenericRequestMonograph"

	// Determine manuscript status
	// MANUSCRIPT_ITEM_TYPES = [
	//  'collection', # @see Firehose::JsonAvailability#set_holdings
	//  'manuscript'  # As seen in Sirsi holdings data.
	// ]

	if contains(doc.WorkTypes, "manuscript") ||
		contains(doc.Medium, "manuscript") ||
		contains(doc.Format, "manuscript") ||
		contains(doc.WorkTypes, "collection") {
		formValue = "GenericRequestManuscript"
	}

	//log.Printf("Solr: %+v", doc)

	// Assign values
	req := aeonRequest{
		Action:      10,
		Form:        20,
		Value:       formValue,
		DocID:       doc.ID,
		Title:       strings.Join(doc.Title, "; "),
		Date:        doc.PublicationDate,
		ISxN:        strings.Join(append(doc.ISBN, doc.ISSN...), ";"),
		Place:       strings.Join(doc.PublishedLocation, "; "),
		Publisher:   strings.Join(doc.PublisherName, "; "),
		Edition:     doc.Edition,
		Issue:       doc.Issue,
		Volume:      doc.Volume,
		Copy:        doc.Copy,
		Description: strings.Join(doc.Description, "; "),
		//Notes:       strings.Join(doc.LocalNotes, ";\r\n"),
		//Location:       doc.Location,
		//CallNumber:  doc.CallNumber,
		//Barcode:     doc.Barcode,
	}
	if len(doc.Author) == 1 {
		req.Author = doc.Author[0]
	} else if len(doc.Author) > 1 {
		req.Author = fmt.Sprintf("%s; ...", doc.Author[0])
	}

	// Notes, Bacode, CallNumber, UserNotes need to be added by client for the specific item!

	query, _ := query.Values(req)

	url := fmt.Sprintf("https://virginia.aeon.atlas-sys.com/logon?%s", query.Encode())

	return url

}

// Appends Aeon request to availability response
func (svc *ServiceContext) removeETASRequestOptions(id string, solrDoc *SolrDocument, result *AvailabilityData) {

	if len(solrDoc.HathiETAS) > 0 {
		log.Printf("ETAS FOUND. Removing request options for %s", id)
		hathiOption := RequestOption{
			Type:           "directLink",
			SignInRequired: false,
			Description: "Use the link above to read this item online through the <a target=\"_blank\" href=\"https://www.library.virginia.edu/services/etas\">Emergency Temporary Access Service.</a>" +
				"<p>Because of U.S. Copyright law, any item made available online through ETAS cannot be also physically circulated. Buttons above reflect any requests that can be made for this item. <a href=\"https://www.library.virginia.edu/news/covid-19/\" target=\"blank\">Read more about digital and physical access during COVID-19.</a></p>",
		}
		if len(solrDoc.URL) > 0 {
			hathiOption.CreateURL = solrDoc.URL[0]
			hathiOption.Label = "Read via HathiTrust"
		}

		// check options
		holdID := -1
		for i, v := range result.Availability.RequestOptions {
			if v.Type == "hold" {
				holdID = i
			}
		}

		// Replace hold option with hathi button
		if holdID != -1 {
			result.Availability.RequestOptions[holdID] = hathiOption
		} else {
			// Append ETAS link even if there is not a hold button
			result.Availability.RequestOptions = append(result.Availability.RequestOptions, hathiOption)
		}

		// Remove non-SC items
		items := []*Item{}
		for _, v := range result.Availability.Items {
			if v.LibraryID == "SPEC-COLL" {
				items = append(items, v)
			}
		}
		result.Availability.Items = items
	}
}

func contains(arr []string, str string) bool {
	if len(arr) == 0 {
		return false
	}

	matcher := regexp.MustCompile("(?i)" + str)
	for _, a := range arr {
		if matcher.MatchString(a) {
			return true
		}
	}
	return false
}

// Use json tags contained in the SolrDocument struct to create the field list for the solr query.
func solrFieldList() string {
	rv := reflect.ValueOf(SolrDocument{})
	t := rv.Type()
	matcher := regexp.MustCompile(`(\w+),`)
	var tags []string
	for i := 0; i < t.NumField(); i++ {
		value := t.Field(i).Tag.Get("json")
		matches := matcher.FindAllStringSubmatch(value, -1)
		if len(matches) > 0 && len(matches[0]) > 0 {
			tags = append(tags, matches[0][1])
		}
	}
	fields := url.QueryEscape(strings.Join(tags, ","))
	return fields
}
