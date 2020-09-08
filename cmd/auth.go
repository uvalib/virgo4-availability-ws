package main

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/uvalib/virgo4-jwt/v4jwt"
)

// authMiddleware is middleware that checks for a user auth token in the
// Authorization header and stores it in the session
func (svc *ServiceContext) authMiddleware(c *gin.Context) {
	tokenStr, err := getBearerToken(c.Request.Header.Get("Authorization"))
	if err != nil {
		log.Printf("Authentication failed: [%s]", err.Error())
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	if tokenStr == "undefined" {
		log.Printf("Authentication failed; bearer token is undefined")
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	log.Printf("Validating JWT auth token...")
	v4Claims, jwtErr := v4jwt.Validate(tokenStr, svc.JWTKey)
	if jwtErr != nil {
		log.Printf("JWT signature for %s is invalid: %s", tokenStr, jwtErr.Error())
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	// add the parsed claims and signed JWT string to the request context so other handlers can access it.
	c.Set("jwt", tokenStr)
	c.Set("claims", v4Claims)
	log.Printf("got bearer token: [%s]: %+v", tokenStr, v4Claims)
	c.Next()
}

// getBearerToken is a helper to extract the token from headers
func getBearerToken(authorization string) (string, error) {
	components := strings.Split(strings.Join(strings.Fields(authorization), " "), " ")

	// must have two components, the first of which is "Bearer", and the second a non-empty token
	if len(components) != 2 || components[0] != "Bearer" || components[1] == "" {
		return "", fmt.Errorf("Invalid Authorization header: [%s]", authorization)
	}

	return components[1], nil
}

// getJWTClaims will pull JWT claims out of the gin context
func getJWTClaims(c *gin.Context) (*v4jwt.V4Claims, error) {
	claims, signedIn := c.Get("claims")
	if signedIn == false {
		return nil, errors.New("No JWT claims found")
	}
	v4Claims, ok := claims.(*v4jwt.V4Claims)
	if !ok {
		return nil, errors.New("Invalid JWT claims found")
	}
	return v4Claims, nil
}
