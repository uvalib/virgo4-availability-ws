package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
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

type reserveItem struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Author     string `json:"author"`
	CallNumber string `json:"callNumber"`
	Library    string `json:"library"`
}

type instructorItems struct {
	InstructorName string        `json:"instructorName"`
	Items          []reserveItem `json:"items"`
}
type courseSearchResponse struct {
	CourseName  string            `json:"courseName"`
	CourseID    string            `json:"courseID"`
	Instructors []instructorItems `json:"instructors"`
}

type courseItems struct {
	CourseName string        `json:"courseName"`
	CourseID   string        `json:"courseID"`
	Items      []reserveItem `json:"items"`
}
type instructorSearchResponse struct {
	InstructorName string        `json:"instructorName"`
	Courses        []courseItems `json:"courses"`
}

type solrReservesResponse struct {
	Response struct {
		Docs     []solrReservesHit `json:"docs,omitempty"`
		NumFound int               `json:"numFound,omitempty"`
	} `json:"response,omitempty"`
}

type solrReservesHit struct {
	ID          string   `json:"id"`
	Title       []string `json:"title_a"`
	Author      []string `json:"work_primary_author_a"`
	CallNumber  []string `json:"call_number_a"`
	Library     []string `json:"library_a"`
	ReserveInfo []string `json:"reserve_id_course_name_a"`
}

func (svc *ServiceContext) searchReserves(c *gin.Context) {
	searchType := c.Query("type")
	if searchType != "instructor_name" && searchType != "course_name" && searchType != "course_id" {
		log.Printf("ERROR: invalid course reserves search type: %s", searchType)
		c.String(http.StatusBadRequest, fmt.Sprintf("%s is not a valid search type", searchType))
		return
	}
	rawQueryStr := c.Query("query")
	queryStr := rawQueryStr
	if strings.Contains(queryStr, "*") == false {
		queryStr += "*"
	}

	claims, err := getJWTClaims(c)
	if err != nil {
		log.Printf("ERROR: search reserves without claims: %s", err.Error())
		c.String(http.StatusForbidden, "not authorized")
		return
	}
	log.Printf("INFO: user [%s] is searching course reserves [%s] for [%s]", claims.UserID, searchType, queryStr)

	fl := url.QueryEscape("id,reserve_id_course_name_a,title_a,work_primary_author_a,call_number_a,library_a")
	queryParam := "reserve_id_a"
	if searchType == "instructor_name" {
		queryParam = "reserve_instructor_tl"
	} else if searchType == "course_name" {
		queryParam = "reserve_course_name_tl"
	} else {
		// course IDs are in all upper case. force query to match
		queryStr = strings.ToUpper(queryStr)
	}
	queryStr = strings.ReplaceAll(queryStr, " ", "+")
	queryParam = fmt.Sprintf("%s:%s", queryParam, queryStr)
	solrURL := fmt.Sprintf("select?fl=%s&q=%s&rows=5000", fl, queryParam)

	respBytes, solrErr := svc.SolrGet(solrURL)
	if solrErr != nil {
		log.Printf("ERROR: solr course reserves search failed: %s", solrErr.Message)
	}
	var solrResp solrReservesResponse
	if err := json.Unmarshal(respBytes, &solrResp); err != nil {
		log.Printf("ERROR: unable to parse solr response: %s.", err.Error())
	}
	log.Printf("INFO: found [%d] matches", solrResp.Response.NumFound)

	if searchType == "instructor_name" {
		reserves := extractInstructorReserves(rawQueryStr, solrResp.Response.Docs)
		c.JSON(http.StatusOK, reserves)
		return
	}

	reserves := extractCourseReserves(searchType, rawQueryStr, solrResp.Response.Docs)
	c.JSON(http.StatusOK, reserves)
}

func extractCourseReserves(tgtType string, tgtCourse string, docs []solrReservesHit) []courseSearchResponse {
	log.Printf("INFO: extract instructor course reserves for %s %s", tgtType, tgtCourse)
	type courseIdentifier struct {
		ID   string
		Name string
	}
	// parse solr response into a map of instructor name -> []courseItems
	raw := make(map[courseIdentifier][]*instructorItems)
	for _, doc := range docs {
		for _, reserve := range doc.ReserveInfo {
			// format: courseID | courseName | instructor
			reserveInfo := strings.Split(reserve, "|")
			courseID := reserveInfo[0]
			courseName := reserveInfo[1]
			instructor := reserveInfo[2]
			courseKeyStr := courseID
			if tgtType == "course_name" {
				courseKeyStr = courseName
			}

			if strings.Index(strings.ToLower(courseKeyStr), strings.ToLower(tgtCourse)) != 0 {
				log.Printf("INFO: skip non-matching %s %s vs %s", tgtType, courseKeyStr, tgtCourse)
				continue
			}

			courseKey := courseIdentifier{ID: courseID, Name: courseName}
			item := reserveItem{ID: doc.ID, Title: doc.Title[0], Author: strings.Join(doc.Author, "; "),
				CallNumber: strings.Join(doc.CallNumber, ", "), Library: strings.Join(doc.Library, ", ")}

			courseInstructors, ok := raw[courseKey]
			if ok == false {
				// first appearance new course, tie item to instructor, add to list and map
				courseInstructors = make([]*instructorItems, 0)
				newInstructor := instructorItems{InstructorName: instructor, Items: make([]reserveItem, 0)}
				newInstructor.Items = append(newInstructor.Items, item)
				courseInstructors = append(courseInstructors, &newInstructor)
				raw[courseKey] = courseInstructors
			} else {
				// course exists, see if instructor exists in the current list of instructors
				var tgtInstructor *instructorItems
				for _, courseInstruct := range courseInstructors {
					if courseInstruct.InstructorName == instructor {
						// instructor already exists, we'll just add items to it
						tgtInstructor = courseInstruct
						break
					}
				}
				if tgtInstructor == nil {
					// add new course to list of courses for this instructor
					tgtInstructor = &instructorItems{InstructorName: instructor, Items: make([]reserveItem, 0)}
					courseInstructors = append(courseInstructors, tgtInstructor)
				}

				// add the item to the course
				tgtInstructor.Items = append(tgtInstructor.Items, item)
			}
		}
	}

	out := make([]courseSearchResponse, 0)
	for key, instructors := range raw {
		csr := courseSearchResponse{CourseName: key.Name, CourseID: key.ID, Instructors: make([]instructorItems, 0)}
		for _, inst := range instructors {
			log.Printf("%s=%+v", key, *inst)
			csr.Instructors = append(csr.Instructors, *inst)
		}
		out = append(out, csr)
	}
	return out
}

func extractInstructorReserves(tgtInstructor string, docs []solrReservesHit) []instructorSearchResponse {
	log.Printf("INFO: extract course course reserves instructor %s", tgtInstructor)
	// parse solr response into a map of instructor name -> []courseItems
	raw := make(map[string][]*courseItems)
	for _, doc := range docs {
		for _, reserve := range doc.ReserveInfo {
			// format: courseID | courseName | instructor
			reserveInfo := strings.Split(reserve, "|")
			courseID := reserveInfo[0]
			courseName := reserveInfo[1]
			instructor := reserveInfo[2]
			if strings.Index(strings.ToLower(instructor), strings.ToLower(tgtInstructor)) != 0 {
				log.Printf("INFO: skip non-matching instructor %s vs %s", instructor, tgtInstructor)
				continue
			}

			item := reserveItem{ID: doc.ID, Title: doc.Title[0], Author: strings.Join(doc.Author, "; "),
				CallNumber: strings.Join(doc.CallNumber, ", "), Library: strings.Join(doc.Library, ", ")}

			instructorCourses, ok := raw[instructor]
			if ok == false {
				// first appearance of instructor. start a new course list, tie it to the instructor
				// and add it all to the raw data map
				instructorCourses = make([]*courseItems, 0)
				newCourse := courseItems{CourseID: courseID, CourseName: courseName, Items: make([]reserveItem, 0)}
				newCourse.Items = append(newCourse.Items, item)
				instructorCourses = append(instructorCourses, &newCourse)
				raw[instructor] = instructorCourses
			} else {
				// instructor  exists, see if this course exists in the instructor courses
				var tgtCourse *courseItems
				for _, course := range instructorCourses {
					if course.CourseID == courseID {
						// course already exists, we'll just add items to it
						tgtCourse = course
						break
					}
				}
				if tgtCourse == nil {
					// add new course to list of courses for this instructor
					tgtCourse = &courseItems{CourseID: courseID, CourseName: courseName, Items: make([]reserveItem, 0)}
					instructorCourses = append(instructorCourses, tgtCourse)
				}

				// add the item to the course
				tgtCourse.Items = append(tgtCourse.Items, item)
			}
		}
	}

	// convert the map to the output response format
	out := make([]instructorSearchResponse, 0)
	for instructor, courses := range raw {
		isr := instructorSearchResponse{InstructorName: instructor, Courses: make([]courseItems, 0)}
		for _, course := range courses {
			isr.Courses = append(isr.Courses, *course)
		}
		out = append(out, isr)
	}

	return out
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
				if solrDoc != nil {
					if (solrDoc.Pool[0] == "video" && contains(solrDoc.Location, "Internet materials")) || contains(solrDoc.Source, "Avalon") {
						log.Printf("INFO: %s is a video", item.ID)
						item.IsVideo = true
						item.Reserve = true
					}
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
