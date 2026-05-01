/*
 * File: tools.go
 * Project: agent
 * Created: 2026-05-01
 *
 * Last Modified: Fri May 01 2026
 * Modified By: Pedro Farias
 */

package agent

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"time"
)

type ToolFunction func(ctx context.Context, input map[string]interface{}) (string, error)

type ToolRegistry struct {
	tools map[string]ToolFunction
	descriptions map[string]string
}

func NewToolRegistry() *ToolRegistry {
	r := &ToolRegistry{
		tools: make(map[string]ToolFunction),
		descriptions: make(map[string]string),
	}
	r.registerDefaultTools()
	return r
}

func (r *ToolRegistry) Register(name string, description string, fn ToolFunction) {
	r.tools[name] = fn
	r.descriptions[name] = description
}

func (r *ToolRegistry) GetToolDescriptions() string {
	desc := ""
	for name, d := range r.descriptions {
		desc += fmt.Sprintf("- %s: %s\n", name, d)
	}
	return desc
}

func (r *ToolRegistry) ExecuteWithRetry(action string, input map[string]interface{}) (string, error) {
	fn, ok := r.tools[action]
	if !ok {
		return "", fmt.Errorf("tool not found: %s", action)
	}

	maxRetries := 3
	var lastErr error
	
	for attempt := 1; attempt <= maxRetries; attempt++ {
		// Enforce timeout per tool
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		
		resultChan := make(chan struct {
			res string
			err error
		}, 1)
		
		go func() {
			res, err := fn(ctx, input)
			resultChan <- struct{res string; err error}{res, err}
		}()
		
		select {
		case <-ctx.Done():
			lastErr = fmt.Errorf("tool execution timed out after 30 seconds")
		case r := <-resultChan:
			if r.err == nil {
				cancel()
				return r.res, nil
			}
			lastErr = r.err
		}
		cancel()
		
		if attempt < maxRetries {
			backoff := time.Duration(attempt * 2) * time.Second
			time.Sleep(backoff)
		}
	}
	
	return "", fmt.Errorf("tool %s failed after %d attempts: %v", action, maxRetries, lastErr)
}

func (r *ToolRegistry) registerDefaultTools() {
	r.Register("http_request", "Make an HTTP request. Input: url, method, body (optional).", func(ctx context.Context, input map[string]interface{}) (string, error) {
		url, ok := input["url"].(string)
		if !ok {
			return "", fmt.Errorf("missing or invalid url")
		}
		method, ok := input["method"].(string)
		if !ok {
			method = "GET"
		}
		
		req, err := http.NewRequestWithContext(ctx, method, url, nil)
		if err != nil {
			return "", err
		}
		
		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode >= 400 {
			return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
		}
		return string(body), nil
	})

	r.Register("file_system", "Read or write file. Input: operation (read|write), path, content (for write).", func(ctx context.Context, input map[string]interface{}) (string, error) {
		op, _ := input["operation"].(string)
		path, _ := input["path"].(string)
		
		if path == "" {
			return "", fmt.Errorf("missing path")
		}
		
		if op == "read" {
			b, err := os.ReadFile(path)
			if err != nil {
				return "", err
			}
			return string(b), nil
		} else if op == "write" {
			content, _ := input["content"].(string)
			err := os.WriteFile(path, []byte(content), 0644)
			if err != nil {
				return "", err
			}
			return "File written successfully", nil
		}
		return "", fmt.Errorf("unknown operation: %s", op)
	})

	r.Register("run_code", "Execute a shell command. Input: command.", func(ctx context.Context, input map[string]interface{}) (string, error) {
		cmdStr, _ := input["command"].(string)
		if cmdStr == "" {
			return "", fmt.Errorf("missing command")
		}
		
		cmd := exec.CommandContext(ctx, "sh", "-c", cmdStr)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("Command failed: %s\nOutput: %s", err.Error(), string(out))
		}
		return string(out), nil
	})
}
