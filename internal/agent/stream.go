/*
 * File: stream.go
 * Project: agent
 * Created: 2026-05-01
 *
 * Last Modified: Fri May 01 2026
 * Modified By: Pedro Farias
 */

package agent

import (
	"sync"
)

type StreamEvent struct {
	Type    string      `json:"type"`
	GoalID  string      `json:"goal_id"`
	Payload interface{} `json:"payload"`
}

var (
	clients map[string][]chan StreamEvent
	mu      sync.Mutex
)

func init() {
	clients = make(map[string][]chan StreamEvent)
}

func Subscribe(goalID string) chan StreamEvent {
	mu.Lock()
	defer mu.Unlock()
	ch := make(chan StreamEvent, 100)
	clients[goalID] = append(clients[goalID], ch)
	return ch
}

func Unsubscribe(goalID string, ch chan StreamEvent) {
	mu.Lock()
	defer mu.Unlock()
	subs := clients[goalID]
	for i, sub := range subs {
		if sub == ch {
			clients[goalID] = append(subs[:i], subs[i+1:]...)
			close(ch)
			break
		}
	}
}

func Broadcast(goalID string, eventType string, payload interface{}) {
	mu.Lock()
	defer mu.Unlock()
	subs := clients[goalID]
	if len(subs) == 0 {
		return
	}
	
	event := StreamEvent{
		Type:    eventType,
		GoalID:  goalID,
		Payload: payload,
	}
	for _, ch := range subs {
		select {
		case ch <- event:
		default:
			// If channel is full, skip
		}
	}
}
