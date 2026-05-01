/*
 * File: db.go
 * Project: mimoproxy
 * Created: 2026-04-29
 */

package services

import (
	"database/sql"
	"fmt"
	"log"
	"mimoproxy/internal/models"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

var DB *sql.DB

func InitDB() {
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "./data/history.db"
	}

	// Ensure directory exists
	dir := filepath.Dir(dbPath)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		_ = os.MkdirAll(dir, 0755)
	}

	var err error
	DB, err = sql.Open("sqlite", dbPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}

	// Create tables
	createTableQuery := `
	CREATE TABLE IF NOT EXISTS messages (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		conversation_id TEXT,
		msg_id TEXT UNIQUE,
		role TEXT,
		content TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_conv ON messages(conversation_id);

	CREATE TABLE IF NOT EXISTS sessions (
		fingerprint TEXT PRIMARY KEY,
		conversation_id TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS agent_states (
		id TEXT PRIMARY KEY,
		goal TEXT,
		status TEXT,
		state_json TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	`
	_, err = DB.Exec(createTableQuery)
	if err != nil {
		log.Fatalf("Failed to create tables: %v", err)
	}

	fmt.Printf("Database initialized at %s\n", dbPath)
}

func SaveMessage(convID, msgID, role, content string) error {
	if convID == "" {
		return nil
	}
	query := `INSERT OR REPLACE INTO messages (conversation_id, msg_id, role, content) VALUES (?, ?, ?, ?)`
	_, err := DB.Exec(query, convID, msgID, role, content)
	return err
}

func GetLocalHistory(convID string) ([]models.Message, error) {
	query := `SELECT role, content FROM messages WHERE conversation_id = ? ORDER BY created_at ASC`
	rows, err := DB.Query(query, convID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []models.Message
	for rows.Next() {
		var m models.Message
		if err := rows.Scan(&m.Role, &m.Content); err != nil {
			return nil, err
		}
		messages = append(messages, m)
	}
	return messages, nil
}

func SaveSession(fingerprint, convID string) error {
	query := `INSERT OR REPLACE INTO sessions (fingerprint, conversation_id) VALUES (?, ?)`
	_, err := DB.Exec(query, fingerprint, convID)
	return err
}

func GetSession(fingerprint string) (string, error) {
	var convID string
	query := `SELECT conversation_id FROM sessions WHERE fingerprint = ?`
	err := DB.QueryRow(query, fingerprint).Scan(&convID)
	if err == nil {
		return convID, nil
	}
	return "", err
}

func FindSessionByMessage(role, content string) (string, error) {
	var convID string
	// Try to find a conversation that starts with this message
	query := `SELECT conversation_id FROM messages WHERE role = ? AND content = ? ORDER BY created_at ASC LIMIT 1`
	err := DB.QueryRow(query, role, content).Scan(&convID)
	if err != nil {
		return "", err
	}
	return convID, nil
}

func SaveAgentState(id, goal, status, stateJson string) error {
	query := `INSERT OR REPLACE INTO agent_states (id, goal, status, state_json, updated_at) VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)`
	_, err := DB.Exec(query, id, goal, status, stateJson)
	return err
}

func GetAgentState(id string) (string, string, string, error) {
	var goal, status, stateJson string
	query := `SELECT goal, status, state_json FROM agent_states WHERE id = ?`
	err := DB.QueryRow(query, id).Scan(&goal, &status, &stateJson)
	return goal, status, stateJson, err
}
