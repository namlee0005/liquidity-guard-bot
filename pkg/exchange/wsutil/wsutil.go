// Package wsutil provides shared WebSocket primitives used by all exchange adapters.
package wsutil

import (
	"context"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// StaleThreshold is the maximum age of the last WS message before the
	// adapter falls back to a REST poll for the order book snapshot.
	StaleThreshold = 5 * time.Second

	writeWait  = 3 * time.Second
	pongWait   = 30 * time.Second
	pingPeriod = 20 * time.Second
)

// Conn wraps gorilla/websocket with heartbeat management.
type Conn struct {
	*websocket.Conn
}

// Dial opens a WebSocket connection with standard timeouts and HTTP headers.
func Dial(ctx context.Context, url string, headers http.Header) (*Conn, error) {
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}
	conn, _, err := dialer.DialContext(ctx, url, headers)
	if err != nil {
		return nil, err
	}
	conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})
	return &Conn{conn}, nil
}

// StartPing sends periodic pings in a goroutine; stops when ctx is cancelled.
func (c *Conn) StartPing(ctx context.Context) {
	go func() {
		t := time.NewTicker(pingPeriod)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				c.SetWriteDeadline(time.Now().Add(writeWait))
				if err := c.WriteMessage(websocket.PingMessage, nil); err != nil {
					return
				}
			}
		}
	}()
}

// IsRetryable returns true for errors that warrant a reconnect (vs. a hard stop).
func IsRetryable(err error) bool {
	if websocket.IsCloseError(err,
		websocket.CloseNormalClosure,
		websocket.CloseGoingAway,
		websocket.CloseAbnormalClosure,
		websocket.CloseServiceRestart,
		websocket.CloseTryAgainLater,
	) {
		return true
	}
	if websocket.IsUnexpectedCloseError(err) {
		return true
	}
	return false
}
