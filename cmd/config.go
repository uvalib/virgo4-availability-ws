package main

import (
	"flag"
	"log"
)

// SolrConfig wraps up the config for solr acess
type SolrConfig struct {
	URL  string
	Core string
}

// ServiceConfig defines all of the v4client service configuration parameters
type ServiceConfig struct {
	Port        int
	ILSAPI      string
	JWTKey      string
	Solr        SolrConfig
	HSILLiadURL string
}

// LoadConfig will load the service configuration from env/cmdline
func loadConfiguration() *ServiceConfig {
	var cfg ServiceConfig
	flag.IntVar(&cfg.Port, "port", 8080, "Service port (default 8080)")
	flag.StringVar(&cfg.JWTKey, "jwtkey", "", "JWT signature key")
	flag.StringVar(&cfg.ILSAPI, "ils", "https://ils-connector.lib.virginia.edu", "ILS Connector API URL")

	// Solr config
	flag.StringVar(&cfg.Solr.URL, "solr", "", "Solr URL for journal browse")
	flag.StringVar(&cfg.Solr.Core, "core", "test_core", "Solr core for journal browse")

	// Illiad communications
	flag.StringVar(&cfg.HSILLiadURL, "hsilliad", "", "HS Illiad API URL")
	flag.Parse()

	if cfg.ILSAPI == "" {
		log.Fatal("ils param is required")
	} else {
		log.Printf("ILS Connector API endpoint: %s", cfg.ILSAPI)
	}
	if cfg.Solr.URL == "" || cfg.Solr.Core == "" {
		log.Fatal("solr and core params are required")
	} else {
		log.Printf("Solr endpoint: %s/%s", cfg.Solr.URL, cfg.Solr.Core)
	}
	if cfg.JWTKey == "" {
		log.Fatal("jwtkey param is required")
	}
	if cfg.HSILLiadURL == "" {
		log.Fatal("hsilliad param is required")
	} else {
		log.Printf("HSIlliad URL: %s", cfg.HSILLiadURL)
	}

	return &cfg
}
