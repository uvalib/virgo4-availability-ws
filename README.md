# Virgo4 Availability service

This is the Virgo4 service used to determine availability information for catalog items

### System Requirements
* GO version 1.12 or greater (mod required)

### Current API

* GET /version : return service version info
* GET /healthcheck : test health of system components; results returned as JSON.
* GET /metrics : returns Prometheus metrics
* GET /api/availability/:id : Get a JSON object containing availability for an item 
