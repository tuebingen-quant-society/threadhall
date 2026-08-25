package realtime_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	rt "github.com/tuebingen-quant-society/threadhall/internal/realtime"
	store "github.com/tuebingen-quant-society/threadhall/internal/store/sqlite"
)

func TestThreeWebSocketsBridgePausedSQLiteReplayExactlyOnceByMembership(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "threadhall.db"), 4)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	seedSocketIntegration(t, db)
	hub := rt.NewHub()
	durable := store.NewReplayStore(db)
	pump, err := rt.NewPump(durable, hub)
	if err != nil {
		t.Fatalf("NewPump: %v", err)
	}
	paused := newPausedReplayStore(durable, 3)
	socket := rt.NewSocket(hub, rt.NewReplayer(paused, pump))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		userID, _ := strconv.ParseInt(request.URL.Query().Get("user_id"), 10, 64)
		afterSeq, _ := strconv.ParseInt(request.URL.Query().Get("after_seq"), 10, 64)
		socket.Serve(w, request, userID, afterSeq)
	}))
	t.Cleanup(func() {
		pump.Close()
		hub.Close()
		server.Close()
		_ = db.Close()
	})

	connections := dialUsers(t, server.URL, []int64{1, 2, 3}, 0)
	select {
	case <-paused.allWaiting:
	case <-time.After(2 * time.Second):
		t.Fatal("three subscribers did not reach paused high-water capture")
	}
	commitIntegrationEvents(t, db)
	pump.Notify(3)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	if err := pump.DrainThrough(ctx, 3); err != nil {
		cancel()
		t.Fatalf("DrainThrough committed events: %v", err)
	}
	cancel()
	close(paused.release)

	for index := 0; index < 2; index++ {
		got := readSequences(t, connections[index], 3)
		assertSequenceList(t, got, []int64{1, 2, 3})
		assertNoEvent(t, connections[index])
	}
	assertNoEvent(t, connections[2])

	reconnected := dialUsers(t, server.URL, []int64{1}, 2)[0]
	defer reconnected.CloseNow()
	assertSequenceList(t, readSequences(t, reconnected, 1), []int64{3})
	assertNoEvent(t, reconnected)
}

func TestPumpSafetyPollDrainsSQLiteCommitWithoutNotify(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "threadhall.db"), 2)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	seedSocketIntegration(t, db)
	hub := rt.NewHub()
	durable := store.NewReplayStore(db)
	pump, err := rt.NewPump(durable, hub)
	if err != nil {
		t.Fatalf("NewPump: %v", err)
	}
	t.Cleanup(func() {
		pump.Close()
		hub.Close()
		_ = db.Close()
	})
	subscription := hub.Subscribe(1, 1)
	memberships, err := durable.Memberships(context.Background(), 1)
	if err != nil {
		t.Fatalf("Memberships: %v", err)
	}
	subscription.SetMemberships(memberships)
	subscription.FinishReplay(1)

	_, err = db.Exec(`INSERT INTO events(
		conversation_id, actor_id, kind, entity_id, payload, created_at
	) VALUES (1, 1, 'message.sent', 2, '{"body":"unnotified"}', 2)`)
	if err != nil {
		t.Fatalf("commit unnotified event: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	delivery, err := subscription.Next(ctx)
	if err != nil || delivery.Event.Seq != 2 {
		t.Fatalf("safety-poll SQLite delivery = (seq %d, %v), want seq 2", delivery.Event.Seq, err)
	}
}

type pausedReplayStore struct {
	*store.ReplayStore
	waitFor    int64
	calls      atomic.Int64
	allWaiting chan struct{}
	release    chan struct{}
}

func newPausedReplayStore(inner *store.ReplayStore, waitFor int64) *pausedReplayStore {
	return &pausedReplayStore{
		ReplayStore: inner, waitFor: waitFor,
		allWaiting: make(chan struct{}), release: make(chan struct{}),
	}
}

func (s *pausedReplayStore) EventBounds(ctx context.Context) (int64, int64, error) {
	call := s.calls.Add(1)
	if call <= s.waitFor {
		if call == s.waitFor {
			close(s.allWaiting)
		}
		select {
		case <-ctx.Done():
			return 0, 0, ctx.Err()
		case <-s.release:
		}
	}
	return s.ReplayStore.EventBounds(ctx)
}

func dialUsers(t *testing.T, serverURL string, userIDs []int64, afterSeq int64) []*websocket.Conn {
	t.Helper()
	type result struct {
		index      int
		connection *websocket.Conn
		err        error
	}
	results := make(chan result, len(userIDs))
	for index, userID := range userIDs {
		go func() {
			target, _ := url.Parse("ws" + serverURL[len("http"):])
			query := target.Query()
			query.Set("user_id", strconv.FormatInt(userID, 10))
			query.Set("after_seq", strconv.FormatInt(afterSeq, 10))
			target.RawQuery = query.Encode()
			connection, _, err := websocket.Dial(context.Background(), target.String(), nil)
			results <- result{index: index, connection: connection, err: err}
		}()
	}
	connections := make([]*websocket.Conn, len(userIDs))
	for range userIDs {
		result := <-results
		if result.err != nil {
			t.Fatalf("Dial user %d: %v", userIDs[result.index], result.err)
		}
		connections[result.index] = result.connection
	}
	return connections
}

func readSequences(t *testing.T, connection *websocket.Conn, count int) []int64 {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	sequences := make([]int64, 0, count)
	for range count {
		_, data, err := connection.Read(ctx)
		if err != nil {
			t.Fatalf("Read event %d: %v", len(sequences), err)
		}
		var event rt.Event
		if err := eventJSON(data, &event); err != nil {
			t.Fatalf("decode event: %v", err)
		}
		sequences = append(sequences, event.Seq)
	}
	return sequences
}

func assertNoEvent(t *testing.T, connection *websocket.Conn) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 175*time.Millisecond)
	defer cancel()
	_, _, err := connection.Read(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("unexpected extra event/error: %v", err)
	}
}

func assertSequenceList(t *testing.T, got, want []int64) {
	t.Helper()
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("sequences = %v, want %v", got, want)
	}
}

func seedSocketIntegration(t *testing.T, db *sql.DB) {
	t.Helper()
	_, err := db.Exec(`
		INSERT INTO users(id, username, password_hash, is_admin, created_at) VALUES
			(1, 'member-one', 'hash', 0, 1),
			(2, 'member-two', 'hash', 0, 1),
			(3, 'outsider', 'hash', 0, 1);
		INSERT INTO conversations(
			id, kind, name, created_by, idempotency_key, created_at
		) VALUES (1, 'private', 'members', 1, 'conversation-1', 1);
		INSERT INTO conversation_members(conversation_id, user_id, joined_at)
			VALUES (1, 1, 1), (1, 2, 1);
		INSERT INTO events(conversation_id, actor_id, kind, entity_id, payload, created_at)
			VALUES (1, 1, 'message.sent', 1, '{"body":"one"}', 1);
	`)
	if err != nil {
		t.Fatalf("seed integration database: %v", err)
	}
}

func commitIntegrationEvents(t *testing.T, db *sql.DB) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO events(
		conversation_id, actor_id, kind, entity_id, payload, created_at
	) VALUES
		(1, 1, 'message.sent', 2, '{"body":"two"}', 2),
		(1, 2, 'message.sent', 3, '{"body":"three"}', 3)`)
	if err != nil {
		t.Fatalf("commit paused events: %v", err)
	}
}

func eventJSON(data []byte, destination any) error {
	return json.Unmarshal(data, destination)
}
