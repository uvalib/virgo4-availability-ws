package main

import (
	"bytes"
	"encoding/csv"
	"io"
	"io/ioutil"
	"log"
)

func (svc *ServiceContext) initMapLookups() {
	log.Printf("Initializing map lookups data...")
	svc.MapLookups = make([]MapLookup, 0)
	svc.Maps = make([]Map, 0)

	// Maps data: ID,URL,NAME
	mapsData, err := ioutil.ReadFile("./data/maps.csv")
	if err != nil {
		log.Printf("ERROR: Unable to read maps data: %s", err.Error())
		return
	}
	csvReader := csv.NewReader(bytes.NewReader(mapsData))
	for {
		line, err := csvReader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Printf("ERROR: Unable to parse maps data: %s", err.Error())
		}
		if line[0] != "ID" {
			mapDat := Map{
				ID:     line[0],
				MapURL: line[1],
				Name:   line[2],
			}
			svc.Maps = append(svc.Maps, mapDat)
		}
	}

	// Lookups: RANGE,LOCATION,MAP
	lookupsData, err := ioutil.ReadFile("./data/map_lookups.csv")
	if err != nil {
		log.Printf("ERROR: Unable to read map lookups data: %s", err.Error())
		return
	}
	csvReader = csv.NewReader(bytes.NewReader(lookupsData))
	for {
		line, err := csvReader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Printf("ERROR: Unable to parse map lookups data: %s", err.Error())
		}
		if line[0] != "RANGE" {
			lookup := MapLookup{
				CallNumberRange: line[0],
				Location:        line[1],
				MapID:           line[2],
			}
			svc.MapLookups = append(svc.MapLookups, lookup)
		}
	}

	log.Printf("Map lookups initialization COMPLETE")
}

func (svc *ServiceContext) addMapInfo(items []*Item) {
	log.Printf("Add map info to items")
	for _, item := range items {
		item.Map.Name = "N/A"
		var lookup *MapLookup
		for _, lu := range svc.MapLookups {
			if lu.Location == item.HomeLocationID {
				lookup = &lu
				break
			}
		}
		if lookup == nil {
			continue
		}

		// if call is an asterisk, location is all thats needed for map
		if lookup.CallNumberRange == "*" {
			match := svc.findMap(lookup.MapID)
			if match != nil {
				item.Map.MapURL = match.MapURL
				item.Map.Name = match.Name
			}
		} else {

		}
	}
}

func (svc *ServiceContext) findMap(id string) *Map {
	var out *Map
	for _, m := range svc.Maps {
		if m.ID == id {
			out = &m
			break
		}
	}
	return out
}
