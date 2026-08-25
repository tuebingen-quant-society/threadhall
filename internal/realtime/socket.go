package realtime

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/coder/websocket"
)

const MaxInboundFrameBytes = 64 << 10

const (
	CloseReasonResyncRequired         = "resync_required"
	CloseReasonFrameTooLarge          = "frame_too_large"
	CloseReasonServerEventsOnly       = "server_events_only"
	CloseReasonConnectionInactive     = "connection_inactive"
	CloseReasonTemporarilyUnavailable = "temporarily_unavailable"
	CloseReasonServerShutdown         = "server_shutdown"
)

type socketConfig struct {
	writeTimeout time.Duration
	pongTimeout  time.Duration
	pingInterval time.Duration
}

var defaultSocketConfig = socketConfig{
	writeTimeout: 10 * time.Second,
	pongTimeout:  60 * time.Second,
	pingInterval: 25 * time.Second,
}

// Socket owns only the WebSocket transport around the transport-neutral hub.
type Socket struct {
	hub      *Hub
	replayer *Replayer
	config   socketConfig
}

func NewSocket(hub *Hub, replayer *Replayer) *Socket {
	return &Socket{hub: hub, replayer: replayer, config: defaultSocketConfig}
}

// Serve upgrades one already-authenticated request for userID.
func (s *Socket) Serve(w http.ResponseWriter, request *http.Request, userID, afterSeq int64) {
	connection, err := websocket.Accept(w, request, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}
	connection.SetReadLimit(-1)
	ctx, cancel := context.WithCancel(context.Background())
	failures := make(chan socketFailure, 1)
	emitFailure := func(failure socketFailure) {
		select {
		case failures <- failure:
		default:
		}
		cancel()
	}
	go readSocket(ctx, connection, emitFailure)
	go pingSocket(ctx, connection, s.config, emitFailure)

	subscription := s.hub.Subscribe(userID, afterSeq)
	defer func() {
		cancel()
		subscription.Close()
		_ = connection.CloseNow()
	}()
	err = s.replayer.CatchUp(ctx, subscription, afterSeq, func(event Event) error {
		encoded, err := json.Marshal(event)
		if err != nil {
			return err
		}
		return writeSocket(ctx, connection, s.config.writeTimeout, encoded)
	})
	if err != nil {
		s.finish(connection, failures, err)
		return
	}

	for {
		delivery, err := s.replayer.Next(ctx, subscription)
		if err != nil {
			s.finish(connection, failures, err)
			return
		}
		if err := writeSocket(ctx, connection, s.config.writeTimeout, delivery.Data); err != nil {
			s.finish(connection, failures, err)
			return
		}
	}
}

func (s *Socket) finish(connection *websocket.Conn, failures <-chan socketFailure, err error) {
	select {
	case failure := <-failures:
		_ = connection.Close(failure.code, failure.reason)
		return
	default:
	}
	switch {
	case errors.Is(err, ErrResyncRequired), errors.Is(err, ErrSlowClient):
		encoded, _ := json.Marshal(struct {
			Type string `json:"type"`
		}{Type: CloseReasonResyncRequired})
		_ = writeSocket(context.Background(), connection, s.config.writeTimeout, encoded)
		_ = connection.Close(websocket.StatusPolicyViolation, CloseReasonResyncRequired)
	case errors.Is(err, ErrHubClosed), errors.Is(err, ErrSubscriptionClosed):
		_ = connection.Close(websocket.StatusGoingAway, CloseReasonServerShutdown)
	case websocket.CloseStatus(err) != -1:
		_ = connection.CloseNow()
	default:
		_ = connection.Close(websocket.StatusTryAgainLater, CloseReasonTemporarilyUnavailable)
	}
}

type socketFailure struct {
	code   websocket.StatusCode
	reason string
}

func readSocket(ctx context.Context, connection *websocket.Conn, fail func(socketFailure)) {
	for {
		_, reader, err := connection.Reader(ctx)
		if err != nil {
			if ctx.Err() == nil && websocket.CloseStatus(err) == -1 {
				fail(socketFailure{websocket.StatusGoingAway, CloseReasonConnectionInactive})
			}
			return
		}
		read, err := io.CopyN(io.Discard, reader, MaxInboundFrameBytes+1)
		if read > MaxInboundFrameBytes {
			fail(socketFailure{websocket.StatusMessageTooBig, CloseReasonFrameTooLarge})
			return
		}
		if err != nil && !errors.Is(err, io.EOF) {
			fail(socketFailure{websocket.StatusUnsupportedData, CloseReasonServerEventsOnly})
			return
		}
		fail(socketFailure{websocket.StatusUnsupportedData, CloseReasonServerEventsOnly})
		return
	}
}

func pingSocket(
	ctx context.Context,
	connection *websocket.Conn,
	config socketConfig,
	fail func(socketFailure),
) {
	for {
		pingCtx, cancel := context.WithTimeout(ctx, config.pongTimeout)
		err := connection.Ping(pingCtx)
		cancel()
		if err != nil {
			if ctx.Err() == nil {
				fail(socketFailure{websocket.StatusGoingAway, CloseReasonConnectionInactive})
			}
			return
		}
		timer := time.NewTimer(config.pingInterval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return
		case <-timer.C:
		}
	}
}

func writeSocket(
	ctx context.Context,
	connection *websocket.Conn,
	timeout time.Duration,
	data []byte,
) error {
	writeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return connection.Write(writeCtx, websocket.MessageText, data)
}
