package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"text/template"

	"github.com/gin-gonic/gin"
)

type availabilityInfo struct {
	Library      string `json:"library"`
	Location     string `json:"location"`
	Availability string `json:"availability"`
	CallNumber   string `json:"callNumber"`
}

type requestItem struct {
	Pool             string             `json:"pool"`
	IsVideo          bool               `json:"isVideo"`
	CatalogKey       string             `json:"catalogKey"`
	CallNumber       string             `json:"callNumber"`
	Title            string             `json:"title"`
	Author           string             `json:"author"`
	Period           string             `json:"period"`
	Notes            string             `json:"notes"`
	AudioLanguage    string             `json:"audioLanguage"`
	Subtitles        string             `json:"subtitles"`
	SubtitleLanguage string             `json:"subtitleLanguage"`
	VirgoURL         string             `json:"-"`
	Availability     []availabilityInfo `json:"-"`
}

type requestParams struct {
	OnBehalfOf      string `json:"onBehalfOf"`
	InstructorName  string `json:"instructorName"`
	InstructorEmail string `json:"instructorEmail"`
	Name            string `json:"name"`
	Email           string `json:"email"`
	Course          string `json:"course"`
	Semester        string `json:"semester"`
	Library         string `json:"library"`
	Period          string `json:"period"`
	LMS             string `json:"lms"`
	OtherLMS        string `json:"otherLMS"`
}

type reserveRequest struct {
	VirgoURL string
	UserID   string         `json:"userID"`
	Request  requestParams  `json:"request"`
	Items    []requestItem  `json:"items"` // these are the items sent from the client
	Video    []*requestItem `json:"-"`     // populated during processing from Items, includes avail
	NonVideo []*requestItem `json:"-"`     // populated during processing from Items, includes avail
	MaxAvail int            `json:"-"`
}

type ilsAvail struct {
	Availability struct {
		Items []ilsItem `json:"items"`
	} `json:"availability"`
}

type ilsItem struct {
	Fields []field `json:"fields"`
}

type field struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type validateResponse struct {
	ID      string `json:"id"`
	Reserve bool   `json:"reserve"`
	IsVideo bool   `json:"is_video"`
}

func (svc *ServiceContext) searchReserves(c *gin.Context) {
	c.String(http.StatusNotImplemented, "not yet")
}

func (svc *ServiceContext) validateCourseReserves(c *gin.Context) {
	var req struct {
		Items []string `json:"items"`
	}
	err := c.ShouldBindJSON(&req)
	if err != nil {
		log.Printf("ERROR: Unable to parse request: %s", err.Error())
		c.String(http.StatusBadRequest, err.Error())
		return
	}

	log.Printf("INFO: validate course reserve items %v", req.Items)
	url := fmt.Sprintf("%s/v4/course_reserves/validate", svc.ILSAPI)
	bodyBytes, ilsErr := svc.ILSConnectorPost(url, req, c.GetString("jwt"))
	if ilsErr != nil {
		c.String(ilsErr.StatusCode, ilsErr.Message)
		return
	}
	var resp []*validateResponse
	if err := json.Unmarshal(bodyBytes, &resp); err != nil {
		log.Printf("ERROR: unable to parse reserve search: %s", err.Error())
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	// if any of the items are flagged as rejected or a non-video by ILS connector, look them
	// up in solr and determine if they are actually a video/streaming video and flag correctly
	// (ils connector doesn't have enout info to determine this completely)
	v4Claims, _ := getJWTClaims(c)
	if v4Claims.CanPlaceReserve {
		log.Printf("INFO: check if any items are type videoReserve")
		for _, item := range resp {
			if item.Reserve == false || item.IsVideo == false {
				solrDoc := svc.getSolrDoc(item.ID)
				if (solrDoc.Pool[0] == "video" && contains(solrDoc.Location, "Internet materials")) || contains(solrDoc.Source, "Avalon") {
					log.Printf("INFO: %s is a video", item.ID)
					item.IsVideo = true
					item.Reserve = true
				}
			}
		}
	}

	c.JSON(http.StatusOK, resp)
}

func (svc *ServiceContext) createCourseReserves(c *gin.Context) {
	log.Printf("Received request to create new course reserves")
	var reserveReq reserveRequest
	err := c.ShouldBindJSON(&reserveReq)
	if err != nil {
		log.Printf("ERROR: Unable to parse request: %s", err.Error())
		c.String(http.StatusBadRequest, err.Error())
		return
	}
	reserveReq.VirgoURL = svc.VirgoURL
	reserveReq.MaxAvail = -1
	reserveReq.Video = make([]*requestItem, 0)
	reserveReq.NonVideo = make([]*requestItem, 0)

	// Iterate thru all of the requested items, pull availability and stuff it into
	// an array based on type. Separate emails will go out for video / non-video
	for idx := range reserveReq.Items {
		itm := &reserveReq.Items[idx]
		itm.VirgoURL = fmt.Sprintf("%s/sources/%s/items/%s", svc.VirgoURL, itm.Pool, itm.CatalogKey)
		svc.getItemAvailability(itm, c.GetString("jwt"))
		if len(itm.Availability) > reserveReq.MaxAvail {
			reserveReq.MaxAvail = len(itm.Availability)
		}
		if itm.IsVideo {
			log.Printf("INFO: %s : %s is a video", itm.CatalogKey, itm.Title)
			reserveReq.Video = append(reserveReq.Video, itm)
		} else {
			log.Printf("INFO: %s : %s is not a video", itm.CatalogKey, itm.Title)
			reserveReq.NonVideo = append(reserveReq.NonVideo, itm)
		}
	}

	funcs := template.FuncMap{"add": func(x, y int) int {
		return x + y
	}, "header": func(cnt int) string {
		out := "|#|Title|Reserve Library|Loan Period|Notes|Virgo URL|"
		for i := 0; i < cnt; i++ {
			out += fmt.Sprintf("Library%d|Location%d|Availability%d|Call Number%d|", i, i, i, i)
		}
		return out
	}, "row": func(idx int, library string, item requestItem) string {
		out := fmt.Sprintf("|%d|%s|%s|%s|%s|%s|",
			idx+1, item.Title, library, item.Period, item.Notes, item.VirgoURL)
		availStr := ""
		for _, avail := range item.Availability {
			availStr += fmt.Sprintf("%s|%s|%s|%s|", avail.Library, avail.Location, avail.Availability, avail.CallNumber)
		}
		out += availStr
		return out
	}}

	templates := [2]string{"reserves.txt", "reserves_video.txt"}
	for _, templateFile := range templates {
		if templateFile == "reserves.txt" && len(reserveReq.NonVideo) == 0 {
			continue
		}
		if templateFile == "reserves_video.txt" && len(reserveReq.Video) == 0 {
			continue
		}
		var renderedEmail bytes.Buffer
		tpl := template.Must(template.New(templateFile).Funcs(funcs).ParseFiles(fmt.Sprintf("templates/%s", templateFile)))
		err = tpl.Execute(&renderedEmail, reserveReq)
		if err != nil {
			log.Printf("ERROR: Unable to render %s: %s", templateFile, err.Error())
			c.String(http.StatusInternalServerError, err.Error())
			return
		}

		log.Printf("Generate SMTP message for %s", templateFile)
		// NOTES for recipient: For any reserve library location other than Law, the email should be sent to
		// svc.CourseReserveEmail with the from address of the patron submitting the request.
		// For Law it should send the email to svc.LawReserveEmail AND the patron
		to := []string{}
		cc := ""
		from := svc.SMTP.Sender
		subjectName := reserveReq.Request.Name
		if reserveReq.Request.Library == "law" {
			log.Printf("The reserve library is law. Send request to law %s and requestor %s from sender %s",
				svc.LawReserveEmail, reserveReq.Request.Email, svc.SMTP.Sender)
			to = append(to, svc.LawReserveEmail)
			to = append(to, reserveReq.Request.Email)
			if reserveReq.Request.InstructorEmail != "" {
				to = append(to, reserveReq.Request.InstructorEmail)
			}
		} else {
			log.Printf("The reserve library is not law.")
			to = append(to, svc.CourseReserveEmail)
			if reserveReq.Request.InstructorEmail != "" {
				from = reserveReq.Request.InstructorEmail
				cc = reserveReq.Request.Email
				subjectName = reserveReq.Request.InstructorName
			} else {
				from = reserveReq.Request.Email
			}
		}

		subject := fmt.Sprintf("%s - %s: %s", reserveReq.Request.Semester, subjectName, reserveReq.Request.Course)
		eRequest := emailRequest{Subject: subject, To: to, CC: cc, From: from, Body: renderedEmail.String()}
		sendErr := svc.sendEmail(&eRequest)
		if sendErr != nil {
			log.Printf("ERROR: Unable to send reserve email: %s", sendErr.Error())
			c.String(http.StatusInternalServerError, sendErr.Error())
			return
		}
	}
	c.String(http.StatusOK, "Reserve emails sent")
}

func (svc *ServiceContext) getItemAvailability(reqItem *requestItem, jwt string) {
	log.Printf("INFO: check if item %s is available for course reserve", reqItem.CatalogKey)
	reqItem.Availability = make([]availabilityInfo, 0)
	availabilityURL := fmt.Sprintf("%s/v4/availability/%s", svc.ILSAPI, reqItem.CatalogKey)
	bodyBytes, ilsErr := svc.ILSConnectorGet(availabilityURL, jwt, svc.HTTPClient)
	if ilsErr != nil {
		log.Printf("WARN: Unable to get availabilty info for reserve %s: %s", reqItem.CatalogKey, ilsErr.Message)
		return
	}

	var availData ilsAvail
	err := json.Unmarshal([]byte(bodyBytes), &availData)
	if err != nil {
		log.Printf("WARN: Invalid ILS Availabilty response for %s: %s", reqItem.CatalogKey, err.Error())
		return
	}

	for _, availItem := range availData.Availability.Items {
		avail := availabilityInfo{}
		for _, field := range availItem.Fields {
			if field.Name == "Library" {
				avail.Library = field.Value
			} else if field.Name == "Availability" {
				avail.Availability = field.Value
			} else if field.Name == "Current Location" {
				avail.Location = field.Value
			} else if field.Name == "Call Number" {
				avail.CallNumber = field.Value
			}
		}
		reqItem.Availability = append(reqItem.Availability, avail)
	}
}
