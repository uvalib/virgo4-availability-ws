package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"
)

// ServiceContext contains common data used by all handlers
type ServiceContext struct {
	Version        string
	ILSAPI         string
	JWTKey         string
	Illiad         IlliadConfig
	Solr           SolrConfig
	HTTPClient     *http.Client
	FastHTTPClient *http.Client
}

// RequestError contains http status code and message for a
// failed ILS Connector request
type RequestError struct {
	StatusCode int
	Message    string
}

// intializeService will initialize the service context based on the config parameters
func intializeService(version string, cfg *ServiceConfig) (*ServiceContext, error) {
	ctx := ServiceContext{Version: version,
		Solr:   cfg.Solr,
		JWTKey: cfg.JWTKey,
		ILSAPI: cfg.ILSAPI,
		Illiad: cfg.Illiad,
	}

	log.Printf("Create HTTP client for external service calls")
	defaultTransport := &http.Transport{
		Dial: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 600 * time.Second,
		}).Dial,
		TLSHandshakeTimeout: 5 * time.Second,
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 100,
	}
	ctx.HTTPClient = &http.Client{
		Transport: defaultTransport,
		Timeout:   10 * time.Second,
	}
	ctx.FastHTTPClient = &http.Client{
		Transport: defaultTransport,
		Timeout:   5 * time.Second,
	}

	return &ctx, nil
}

// ignoreFavicon is a dummy to handle browser favicon requests without warnings
func (svc *ServiceContext) ignoreFavicon(c *gin.Context) {
}

// GetVersion reports the version of the serivce
func (svc *ServiceContext) getVersion(c *gin.Context) {
	build := "unknown"
	// cos our CWD is the bin directory
	files, _ := filepath.Glob("../buildtag.*")
	if len(files) == 1 {
		build = strings.Replace(files[0], "../buildtag.", "", 1)
	}

	vMap := make(map[string]string)
	vMap["version"] = svc.Version
	vMap["build"] = build
	c.JSON(http.StatusOK, vMap)
}

// HealthCheck reports the health of the server
func (svc *ServiceContext) healthCheck(c *gin.Context) {
	log.Printf("Got healthcheck request")
	type hcResp struct {
		Healthy bool   `json:"healthy"`
		Message string `json:"message,omitempty"`
		Version int    `json:"version,omitempty"`
	}
	hcMap := make(map[string]hcResp)

	if svc.ILSAPI != "" {
		apiURL := fmt.Sprintf("%s/version", svc.ILSAPI)
		resp, err := svc.FastHTTPClient.Get(apiURL)
		if resp != nil {
			defer resp.Body.Close()
		}
		if err != nil {
			log.Printf("ERROR: Failed response from ILS Connector PING: %s - %s", err.Error(), svc.ILSAPI)
			hcMap["ils_connector"] = hcResp{Healthy: false, Message: err.Error()}
		} else {
			hcMap["ils_connector"] = hcResp{Healthy: true}
		}
	}

	respBytes, illErr := svc.ILLiadRequest("GET", "SystemInfo/SecurePlatformVersion", nil)
	if illErr != nil {
		log.Printf("ERROR: Failed response from ILLiad PING: %s", illErr.Message)
		hcMap["illiad"] = hcResp{Healthy: false, Message: illErr.Message}
	} else {
		hcMap["illiad"] = hcResp{Healthy: true}
		log.Printf("ILLiad version: %s", respBytes)
	}

	c.JSON(http.StatusOK, hcMap)
}

type solrRequestParams struct {
	Rows int      `json:"rows"`
	Fq   []string `json:"fq,omitempty"`
	Q    string   `json:"q,omitempty"`
}

type solrRequestFacet struct {
	Type  string `json:"type,omitempty"`
	Field string `json:"field,omitempty"`
	Sort  string `json:"sort,omitempty"`
	Limit int    `json:"limit,omitempty"`
}

type solrRequest struct {
	Params solrRequestParams           `json:"params"`
	Facets map[string]solrRequestFacet `json:"facet,omitempty"`
}

// ILLiadRequest sends a GET/PUT/POST request to ILLiad and returns results
func (svc *ServiceContext) ILLiadRequest(verb string, url string, data interface{}) ([]byte, *RequestError) {
	log.Printf("ILLiad  %s request: %s, %+v", verb, url, data)
	illiadURL := fmt.Sprintf("%s/%s", svc.Illiad.URL, url)

	var illReq *http.Request
	if data != nil {
		b, _ := json.Marshal(data)
		illReq, _ = http.NewRequest(verb, illiadURL, bytes.NewBuffer(b))
	} else {
		illReq, _ = http.NewRequest(verb, illiadURL, nil)
	}

	illReq.Header.Add("Content-Type", "application/json")
	illReq.Header.Add("ApiKey", svc.Illiad.APIKey)

	startTime := time.Now()
	rawResp, rawErr := svc.HTTPClient.Do(illReq)
	resp, err := handleAPIResponse(url, rawResp, rawErr)
	elapsedNanoSec := time.Since(startTime)
	elapsedMS := int64(elapsedNanoSec / time.Millisecond)

	if err != nil {
		if shouldLogAsError(err.StatusCode) {
			log.Printf("ERROR: Failed response from ILLiad %s %s - %d:%s. Elapsed Time: %d (ms)",
				verb, url, err.StatusCode, err.Message, elapsedMS)
		} else {
			log.Printf("INFO: Response from ILLiad %s %s - %d:%s. Elapsed Time: %d (ms)",
				verb, url, err.StatusCode, err.Message, elapsedMS)
		}
	} else {
		log.Printf("Successful response from ILLiad %s %s. Elapsed Time: %d (ms)", verb, url, elapsedMS)
	}
	return resp, err
}

// ILSConnectorGet sends a GET request to the ILS connector and returns the response
func (svc *ServiceContext) ILSConnectorGet(url string, jwt string, httpClient *http.Client) ([]byte, *RequestError) {

	logURL := sanitizeURL(url)
	log.Printf("ILS Connector GET request: %s, timeout  %.0f sec", logURL, httpClient.Timeout.Seconds())
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", jwt))

	startTime := time.Now()
	rawResp, rawErr := httpClient.Do(req)
	resp, err := handleAPIResponse(logURL, rawResp, rawErr)
	elapsedNanoSec := time.Since(startTime)
	elapsedMS := int64(elapsedNanoSec / time.Millisecond)

	if err != nil {
		if shouldLogAsError(err.StatusCode) {
			log.Printf("ERROR: Failed response from ILS GET %s - %d:%s. Elapsed Time: %d (ms)",
				logURL, err.StatusCode, err.Message, elapsedMS)
		} else {
			log.Printf("INFO: Response from ILS GET %s - %d:%s. Elapsed Time: %d (ms)",
				logURL, err.StatusCode, err.Message, elapsedMS)
		}
	} else {
		log.Printf("Successful response from ILS GET %s. Elapsed Time: %d (ms)", logURL, elapsedMS)
	}
	return resp, err
}

// ILSConnectorPost sends a POST to the ILS connector and returns results
func (svc *ServiceContext) ILSConnectorPost(url string, values interface{}, jwt string) ([]byte, *RequestError) {
	log.Printf("ILS Connector POST request: %s", url)
	startTime := time.Now()
	b, _ := json.Marshal(values)
	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(b))
	req.Header.Add("Content-type", "application/json")
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", jwt))
	httpClient := svc.HTTPClient
	rawResp, rawErr := httpClient.Do(req)
	resp, err := handleAPIResponse(url, rawResp, rawErr)
	elapsedNanoSec := time.Since(startTime)
	elapsedMS := int64(elapsedNanoSec / time.Millisecond)

	if err != nil {
		log.Printf("ERROR: Failed response from ILS POST %s - %d:%s. Elapsed Time: %d (ms)",
			url, err.StatusCode, err.Message, elapsedMS)
	} else {
		log.Printf("Successful response from ILS POST %s. Elapsed Time: %d (ms)", url, elapsedMS)
	}
	return resp, err
}

// ILSConnectorDelete sends a DELETE request to the ILS connector and returns the response
func (svc *ServiceContext) ILSConnectorDelete(url string, jwt string) ([]byte, *RequestError) {
	log.Printf("ILS Connector DELETE request: %s", url)
	req, _ := http.NewRequest("DELETE", url, nil)
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", jwt))

	startTime := time.Now()
	rawResp, rawErr := svc.HTTPClient.Do(req)
	resp, err := handleAPIResponse(url, rawResp, rawErr)
	elapsedNanoSec := time.Since(startTime)
	elapsedMS := int64(elapsedNanoSec / time.Millisecond)

	if err != nil {
		log.Printf("ERROR: Failed response from ILS DELETE %s - %d:%s. Elapsed Time: %d (ms)",
			url, err.StatusCode, err.Message, elapsedMS)
	} else {
		log.Printf("Successful response from ILS DELETE %s. Elapsed Time: %d (ms)", url, elapsedMS)
	}
	return resp, err
}

// SolrGet sends a GET request to solr and returns the response
func (svc *ServiceContext) SolrGet(query string) ([]byte, *RequestError) {
	url := fmt.Sprintf("%s/%s/%s", svc.Solr.URL, svc.Solr.Core, query)
	log.Printf("Solr GET request: %s", url)
	startTime := time.Now()
	rawResp, rawErr := svc.FastHTTPClient.Get(url)
	resp, err := handleAPIResponse(url, rawResp, rawErr)
	elapsedNanoSec := time.Since(startTime)
	elapsedMS := int64(elapsedNanoSec / time.Millisecond)

	if err != nil {
		log.Printf("ERROR: Failed response from Solr GET %s - %d:%s. Elapsed Time: %d (ms)",
			url, err.StatusCode, err.Message, elapsedMS)
	} else {
		log.Printf("Successful response from Solr GET %s. Elapsed Time: %d (ms)", url, elapsedMS)
	}
	return resp, err
}

// SolrPost sends a json POST request to solr and returns the response
func (svc *ServiceContext) SolrPost(query string, jsonReq interface{}) ([]byte, *RequestError) {
	url := fmt.Sprintf("%s/%s/%s", svc.Solr.URL, svc.Solr.Core, query)

	jsonBytes, jsonErr := json.Marshal(jsonReq)
	if jsonErr != nil {
		resp, err := handleAPIResponse(url, nil, jsonErr)
		return resp, err
	}

	req, reqErr := http.NewRequest("POST", url, bytes.NewBuffer(jsonBytes))
	if reqErr != nil {
		resp, err := handleAPIResponse(url, nil, reqErr)
		return resp, err
	}

	req.Header.Set("Content-Type", "application/json")

	log.Printf("Solr POST request: %s", url)
	startTime := time.Now()
	rawResp, rawErr := svc.FastHTTPClient.Do(req)
	resp, err := handleAPIResponse(url, rawResp, rawErr)
	elapsedNanoSec := time.Since(startTime)
	elapsedMS := int64(elapsedNanoSec / time.Millisecond)

	if err != nil {
		log.Printf("ERROR: Failed response from Solr POST %s - %d:%s. Elapsed Time: %d (ms)",
			url, err.StatusCode, err.Message, elapsedMS)
	} else {
		log.Printf("Successful response from Solr POST %s. Elapsed Time: %d (ms)", url, elapsedMS)
	}
	return resp, err
}

func handleAPIResponse(logURL string, resp *http.Response, err error) ([]byte, *RequestError) {
	if err != nil {
		status := http.StatusBadRequest
		errMsg := err.Error()
		if strings.Contains(err.Error(), "Timeout") {
			status = http.StatusRequestTimeout
			errMsg = fmt.Sprintf("%s timed out", logURL)
		} else if strings.Contains(err.Error(), "connection refused") {
			status = http.StatusServiceUnavailable
			errMsg = fmt.Sprintf("%s refused connection", logURL)
		}
		return nil, &RequestError{StatusCode: status, Message: errMsg}
	} else if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		defer resp.Body.Close()
		bodyBytes, _ := ioutil.ReadAll(resp.Body)
		status := resp.StatusCode
		errMsg := string(bodyBytes)
		return nil, &RequestError{StatusCode: status, Message: errMsg}
	}

	defer resp.Body.Close()
	bodyBytes, _ := ioutil.ReadAll(resp.Body)
	return bodyBytes, nil
}

// do we log this http response as an error or is it expected under normal circumstances
func shouldLogAsError(httpStatus int) bool {
	return httpStatus != http.StatusOK && httpStatus != http.StatusNotFound
}

// sanitize a url for logging by removing the customer PIN
func sanitizeURL(url string) string {

	// URL contains the user PIN
	ix := strings.Index(url, "pin=")

	// replace everything after
	if ix >= 0 {
		return url[0:ix] + "pin=SECRET"
	}

	return url
}
