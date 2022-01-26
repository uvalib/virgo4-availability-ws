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

// SMTPConfig wraps up all of the smpt configuration
type SMTPConfig struct {
	Host    string
	Port    int
	User    string
	Pass    string
	Sender  string
	DevMode bool
}

// ServiceConfig defines all of the v4client service configuration parameters
type ServiceConfig struct {
	Port               int
	VirgoURL           string
	ILSAPI             string
	JWTKey             string
	Solr               SolrConfig
	HSILLiadURL        string
	CourseReserveEmail string
	LawReserveEmail    string
	SMTP               SMTPConfig
}

// LoadConfig will load the service configuration from env/cmdline
func loadConfiguration() *ServiceConfig {
	var cfg ServiceConfig
	flag.IntVar(&cfg.Port, "port", 8080, "Service port (default 8080)")
	flag.StringVar(&cfg.VirgoURL, "virgo", "https://search.virginia.edu", "URL to Virgo")
	flag.StringVar(&cfg.JWTKey, "jwtkey", "", "JWT signature key")
	flag.StringVar(&cfg.ILSAPI, "ils", "https://ils-connector.lib.virginia.edu", "ILS Connector API URL")
	flag.StringVar(&cfg.CourseReserveEmail, "cremail", "", "Email recipient for course reserves requests")
	flag.StringVar(&cfg.LawReserveEmail, "lawemail", "", "Law Email recipient for course reserves requests")

	// Solr config
	flag.StringVar(&cfg.Solr.URL, "solr", "", "Solr URL for journal browse")
	flag.StringVar(&cfg.Solr.Core, "core", "test_core", "Solr core for journal browse")

	// SMTP settings
	flag.StringVar(&cfg.SMTP.Host, "smtphost", "", "SMTP Host")
	flag.IntVar(&cfg.SMTP.Port, "smtpport", 0, "SMTP Port")
	flag.StringVar(&cfg.SMTP.User, "smtpuser", "", "SMTP User")
	flag.StringVar(&cfg.SMTP.Pass, "smtppass", "", "SMTP Password")
	flag.StringVar(&cfg.SMTP.Sender, "smtpsender", "virgo4@virginia.edu", "SMTP sender email")
	flag.BoolVar(&cfg.SMTP.DevMode, "stubsmtp", false, "Log email insted of sending (dev mode)")

	// Illiad communications
	flag.StringVar(&cfg.HSILLiadURL, "hsilliad", "", "HS Illiad API URL")
	flag.Parse()

	// Fatal error for missing required params
	if cfg.ILSAPI == "" {
		log.Fatal("ils param is required")
	}
	if cfg.Solr.URL == "" || cfg.Solr.Core == "" {
		log.Fatal("solr and core params are required")
	}
	if cfg.JWTKey == "" {
		log.Fatal("jwtkey param is required")
	}
	if cfg.HSILLiadURL == "" {
		log.Fatal("hsilliad param is required")
	}
	if cfg.CourseReserveEmail == "" {
		log.Fatal("cremail param is required")
	}
	if cfg.LawReserveEmail == "" {
		log.Fatal("lawemail param is required")
	}

	log.Printf("[CONFIG] port          = [%d]", cfg.Port)
	log.Printf("[CONFIG] virgo         = [%s]", cfg.VirgoURL)
	log.Printf("[CONFIG] ils           = [%s]", cfg.ILSAPI)
	log.Printf("[CONFIG] solr          = [%s]", cfg.Solr.URL)
	log.Printf("[CONFIG] core          = [%s]", cfg.Solr.Core)
	if cfg.SMTP.Host != "" {
		log.Printf("[CONFIG] smtphost      = [%s]", cfg.SMTP.Host)
		log.Printf("[CONFIG] smtpport      = [%d]", cfg.SMTP.Port)
	}
	if cfg.SMTP.User != "" {
		log.Printf("[CONFIG] smtpuser      = [%s]", cfg.SMTP.User)
	}
	log.Printf("[CONFIG] smtpsender    = [%s]", cfg.SMTP.Sender)
	log.Printf("[CONFIG] stubsmtp      = [%t]", cfg.SMTP.DevMode)
	log.Printf("[CONFIG] cremail       = [%s]", cfg.CourseReserveEmail)
	log.Printf("[CONFIG] lawemail      = [%s]", cfg.LawReserveEmail)
	log.Printf("[CONFIG] hsilliad      = [%s]", cfg.HSILLiadURL)

	return &cfg
}
