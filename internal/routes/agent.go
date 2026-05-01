/*
 * File: agent.go
 * Project: routes
 * Created: 2026-05-01
 *
 * Last Modified: Fri May 01 2026
 * Modified By: Pedro Farias
 */

package routes

import (
	"encoding/json"
	"fmt"
	"mimoproxy/internal/agent"
	"mimoproxy/internal/utils"
	"net/http"

	"github.com/gin-gonic/gin"
)

func RegisterAgentRoutes(r *gin.Engine) {
	agentGroup := r.Group("/v1/agent")
	{
		agentGroup.POST("/run", handleRunAgent)
		agentGroup.GET("/status/:id", handleGetAgentStatus)
		agentGroup.GET("/stream/:id", handleStreamAgent)
	}
}

type AgentRunRequest struct {
	GoalID   string `json:"goal_id"`
	Goal     string `json:"goal" binding:"required"`
	MaxSteps int    `json:"max_steps"`
}

func handleRunAgent(c *gin.Context) {
	var req AgentRunRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request: " + err.Error()})
		return
	}

	if req.GoalID == "" {
		req.GoalID = "goal_" + utils.GenerateID()
	}

	maxSteps := req.MaxSteps
	if maxSteps <= 0 {
		maxSteps = 10
	}

	// Run in background for asynchrony
	go func() {
		agent.RunAgentLoop(req.GoalID, req.Goal, maxSteps)
	}()

	c.JSON(http.StatusAccepted, gin.H{
		"message": "Agent loop started",
		"goal_id": req.GoalID,
	})
}

func handleGetAgentStatus(c *gin.Context) {
	id := c.Param("id")
	
	state, err := agent.LoadOrInitializeState(id, "")
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "State not found or failed to load"})
		return
	}

	c.JSON(http.StatusOK, state)
}

func handleStreamAgent(c *gin.Context) {
	id := c.Param("id")
	
	// Ensure SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	ch := agent.Subscribe(id)
	defer agent.Unsubscribe(id, ch)

	// Send initial connection event
	c.Writer.WriteString(fmt.Sprintf("data: %s\n\n", `{"type":"connected"}`))
	c.Writer.Flush()

	for {
		select {
		case <-c.Request.Context().Done():
			return
		case event, ok := <-ch:
			if !ok {
				return
			}
			eventBytes, _ := json.Marshal(event)
			c.Writer.WriteString(fmt.Sprintf("data: %s\n\n", string(eventBytes)))
			c.Writer.Flush()
			
			// If finished, close connection automatically
			if event.Type == "finished" {
				return
			}
		}
	}
}

