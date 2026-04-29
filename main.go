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

		modelListHtml := "<li>mimo-v2.5-pro (fallback)</li>"
		auth := services.GetSelectedAuth()
		headers := services.GetOfficialHeaders(auth, nil)
		client := &http.Client{Timeout: 5 * time.Second}
		req, _ := http.NewRequest("GET", "https://aistudio.xiaomimimo.com/open-apis/bot/config", nil)
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		resp, err := client.Do(req)
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

		html := fmt.Sprintf(`
        <html>
            <head>
                <title>Xiaomi Mimo Proxy (Go)</title>
                <style>
                    body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; line-height: 1.6; max-width: 800px; margin: 0 auto; padding: 2rem; background: #f4f4f9; }
                    .card { background: white; padding: 2rem; border-radius: 8px; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }
                    h1 { color: #333; margin-top: 0; }
                    code { background: #eee; padding: 0.2rem 0.4rem; border-radius: 4px; font-size: 0.9em; }
                    .status { display: inline-block; width: 12px; height: 12px; border-radius: 50%; background: #22c55e; margin-right: 8px; }
                    ul { padding-left: 1.2rem; }
                </style>
            </head>
            <body>
                <div class="card">
                    <h1><span class="status"></span>Mimo AI Proxy (Go)</h1>
                    <p>Uptime: <strong>%.0fs</strong></p>
                    <hr/>
                    <h3>Available Endpoints:</h3>
                    <ul>
                        <li><code>POST /v1/chat/completions</code> - OpenAI Compatible</li>
                        <li><code>GET /v1/models</code> - Dynamic Model List</li>
                        <li><code>GET /health</code> - System Health</li>
                    </ul>
                    <hr/>
                    <h3>Active Models:</h3>
                    <ul>%s</ul>
                    <hr/>
                    <h3>Performance:</h3>
                    <p>Avg Upstream Latency: <strong>%s</strong></p>
                    <hr/>
                    <h3>Token Usage Stats:</h3>
                    <ul>%s</ul>
                </div>
            </body>
        </html>
    `, time.Since(startTime).Seconds(), modelListHtml, avgTimeStr, statsHtml)
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(html))
	})

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "ok",
			"uptime": time.Since(startTime).Seconds(),
		})
	})

	// Mount chat routes
	routes.RegisterChatRoutes(r, middleware.ValidateApiKey())

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
