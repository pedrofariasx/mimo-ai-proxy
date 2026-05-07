/*
 * File: main.go
 * Project: mimoproxy
 * Author: Pedro Farias
 * Created: 2026-04-29
 */

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"mimoproxy/internal/middleware"
	"mimoproxy/internal/routes"
	"mimoproxy/internal/services"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

var startTime time.Time

func init() {
	startTime = time.Now()
}

func main() {
	_ = godotenv.Load()
	
	// Initialize local database
	services.InitDB()

	r := gin.New()
	
	// Set up templates
	r.SetFuncMap(template.FuncMap{
		"safe": func(s string) template.HTML {
			return template.HTML(s)
		},
	})
	r.LoadHTMLGlob("templates/*")

	// Set Max Request Body Size to 100MB
	r.Use(func(c *gin.Context) {
		// Increase limit to 100MB
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 100<<20)
		c.Next()
	})
	r.Use(gin.Recovery())

	// Custom Logger
	r.Use(func(c *gin.Context) {
		if c.Request.URL.Path != "/health" {
			log.Printf("[%s] %s %s", time.Now().Format(time.RFC3339), c.Request.Method, c.Request.URL.Path)
		}
		c.Next()
	})

	// Configurable CORS
	r.Use(func(c *gin.Context) {
		origin := os.Getenv("CORS_ORIGIN")
		if origin == "" {
			origin = "*"
		}
		c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With, Accept, X-Timezone")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	// Root route - Dashboard
	r.GET("/", func(c *gin.Context) {
		tokenStats, tokenUsage, responseTimes := routes.GetStats()
		
		var statsItems []string
		for token, count := range tokenStats {
			displayToken := token
			if len(token) > 10 {
				displayToken = token[:10] + "..."
			}
			usage := tokenUsage[token]
			statsItems = append(statsItems, fmt.Sprintf("<li>Token <code>%s</code>: <strong>%d</strong> reqs, <strong>%.1fk</strong> tokens</li>", displayToken, count, float64(usage)/1000.0))
		}
		statsHtml := strings.Join(statsItems, "")
		if statsHtml == "" {
			statsHtml = "<li>No requests processed yet.</li>"
		}

		var avgTimeStr string = "N/A"
		if len(responseTimes) > 0 {
			var sum int64
			for _, t := range responseTimes {
				sum += t
			}
			avgTimeStr = fmt.Sprintf("%dms", sum/int64(len(responseTimes)))
		}

		modelListHtml := "<li>API models unavailable</li>"
		auth := services.GetSelectedAuth()
		headers := services.GetOfficialHeaders(auth, nil)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		req, _ := http.NewRequestWithContext(ctx, "GET", "https://aistudio.xiaomimimo.com/open-apis/bot/config", nil)
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		resp, err := services.GlobalHTTPClient.Do(req)
		if err == nil && resp.StatusCode == http.StatusOK {
			var result struct {
				Code int `json:"code"`
				Data struct {
					ModelConfigList []struct {
						Model   string `json:"model"`
						EnIntro string `json:"enIntro"`
					} `json:"modelConfigList"`
				} `json:"data"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&result); err == nil && result.Code == 0 {
				var modelItems []string
				for _, m := range result.Data.ModelConfigList {
					modelItems = append(modelItems, fmt.Sprintf("<li><code>%s</code> - %s</li>", m.Model, m.EnIntro))
				}
				modelListHtml = strings.Join(modelItems, "")
			}
		}

		c.HTML(http.StatusOK, "dashboard.html", gin.H{
			"Uptime":     fmt.Sprintf("%.0f", time.Since(startTime).Seconds()),
			"ModelList":  modelListHtml,
			"AvgLatency": avgTimeStr,
			"Stats":      statsHtml,
		})
	})

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "ok",
			"uptime": time.Since(startTime).Seconds(),
		})
	})

	// Mount chat routes
	routes.RegisterChatRoutes(r, middleware.ValidateApiKey())
	routes.RegisterAgentRoutes(r)

	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	// For Docker environments, it's safer to bind to 0.0.0.0 explicitly
	address := "0.0.0.0:" + port

	srv := &http.Server{
		Addr:           address,
		Handler:        r,
		MaxHeaderBytes: 1 << 20, // 1MB headers
		ReadTimeout:    600 * time.Second,
		WriteTimeout:   600 * time.Second,
	}

	go func() {
		log.Printf("Mimo Proxy listening on %s", address)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %s\n", err)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server with
	// a timeout of 5 seconds.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutdown Server ...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal("Server Shutdown:", err)
	}
	log.Println("Server exiting")
}
