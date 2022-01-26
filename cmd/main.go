package main

import (
	"fmt"
	"log"

	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
)

// Version of the service
const version = "1.1.0"

func main() {
	log.Printf("===> V4 availability service staring up <===")

	// Get config params and use them to init service context. Any issues are fatal
	cfg := loadConfiguration()
	svc, err := intializeService(version, cfg)
	if err != nil {
		log.Fatal(err.Error())
	}

	log.Printf("Setup routes...")
	gin.SetMode(gin.ReleaseMode)
	gin.DisableConsoleColor()
	router := gin.Default()
	router.Use(gzip.Gzip(gzip.DefaultCompression))
	corsCfg := cors.DefaultConfig()
	corsCfg.AllowAllOrigins = true
	corsCfg.AllowCredentials = true
	corsCfg.AddAllowHeaders("Authorization")
	router.Use(cors.New(corsCfg))

	router.GET("/", svc.getVersion)
	router.GET("/favicon.ico", svc.ignoreFavicon)
	router.GET("/version", svc.getVersion)
	router.GET("/healthcheck", svc.healthCheck)
	router.GET("/item/:id", svc.authMiddleware, svc.getAvailability)

	// course reserves
	router.POST("/reserves", svc.authMiddleware, svc.createCourseReserves)
	router.POST("/reserves/validate", svc.authMiddleware, svc.validateCourseReserves)
	router.GET("/reserves/search", svc.authMiddleware, svc.searchReserves)

	portStr := fmt.Sprintf(":%d", cfg.Port)
	log.Printf("Start service v%s on port %s", version, portStr)
	log.Fatal(router.Run(portStr))
}
