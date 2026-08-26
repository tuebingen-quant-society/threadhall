// Command threadhall runs the Threadhall server.
package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"database/sql"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/tuebingen-quant-society/threadhall/internal/app"
	"github.com/tuebingen-quant-society/threadhall/internal/auth"
	"github.com/tuebingen-quant-society/threadhall/internal/config"
	"github.com/tuebingen-quant-society/threadhall/internal/conversation"
	"github.com/tuebingen-quant-society/threadhall/internal/httpapi"
	"github.com/tuebingen-quant-society/threadhall/internal/message"
	"github.com/tuebingen-quant-society/threadhall/internal/realtime"
	store "github.com/tuebingen-quant-society/threadhall/internal/store/sqlite"
	"golang.org/x/term"
)

var version = "dev"

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout); err != nil {
		log.Fatal(err)
	}
}

func run(arguments []string, stdin *os.File, stdout io.Writer) error {
	if len(arguments) == 1 && arguments[0] == "version" {
		_, err := fmt.Fprintln(stdout, version)
		return err
	}
	if len(arguments) == 0 {
		return fmt.Errorf("usage: threadhall serve [options] | threadhall bootstrap-admin [options] | threadhall version")
	}
	switch arguments[0] {
	case "serve":
		return serve(arguments[1:])
	case "bootstrap-admin":
		return bootstrapAdmin(arguments[1:], stdin, stdout)
	case "bootstrap-agent":
		return bootstrapAgent(arguments[1:], stdout)
	case "set-agent-policy":
		return setAgentPolicy(arguments[1:])
	default:
		return fmt.Errorf("unknown command %q", arguments[0])
	}
}

func serve(arguments []string) error {
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	address := flags.String("addr", ":8080", "HTTP listen address")
	statePath := flags.String("state-path", "", "required SQLite state path")
	publicURL := flags.String("public-url", "", "required public HTTP or HTTPS URL")
	production := flags.Bool("production", false, "enforce production security rules")
	secureCookies := flags.Bool("secure-cookies", false, "mark browser cookies Secure")
	writerQueue := flags.Int("writer-queue", 0, "bounded SQLite writer queue size")
	readConnections := flags.Int("read-connections", 0, "bounded SQLite connection count")
	if err := flags.Parse(arguments); err != nil {
		return err
	}

	cfg := config.Config{
		StatePath:       *statePath,
		PublicURL:       *publicURL,
		Production:      *production,
		SecureCookies:   *secureCookies,
		WriterQueueSize: *writerQueue,
		ReadConnections: *readConnections,
	}
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}
	db, err := store.Open(cfg.StatePath, cfg.ReadConnections)
	if err != nil {
		return fmt.Errorf("start persistence: %w", err)
	}
	defer db.Close()
	writer, err := store.NewWriter(db, cfg.WriterQueueSize)
	if err != nil {
		return fmt.Errorf("start persistence writer: %w", err)
	}
	defer writer.Close()
	handler, err := newServerHandler(db, writer, cfg)
	if err != nil {
		return err
	}
	defer handler.Close()
	server := &http.Server{Addr: *address, Handler: handler}
	log.Printf("Threadhall %s listening on %s", version, *address)
	return server.ListenAndServe()
}

type serverHandler struct {
	http.Handler
	pump *realtime.Pump
	hub  *realtime.Hub
	once sync.Once
}

func (h *serverHandler) Close() {
	h.once.Do(func() {
		h.pump.Close()
		h.hub.Close()
	})
}

func newServerHandler(db *sql.DB, writer *store.Writer, cfg config.Config) (*serverHandler, error) {
	authService, err := auth.NewService(store.NewAuthStore(db, writer), time.Now, rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("start authentication: %w", err)
	}
	conversationService, err := conversation.NewService(store.NewConversationStore(db, writer), time.Now)
	if err != nil {
		return nil, fmt.Errorf("start conversations: %w", err)
	}
	messageService, err := message.NewService(store.NewMessageStore(db, writer), time.Now)
	if err != nil {
		return nil, fmt.Errorf("start messages: %w", err)
	}
	replayStore := store.NewReplayStore(db)
	hub := realtime.NewHub()
	pump, err := realtime.NewPump(replayStore, hub)
	if err != nil {
		return nil, fmt.Errorf("start realtime event pump: %w", err)
	}
	socket := realtime.NewSocket(hub, realtime.NewReplayer(replayStore, pump))
	agentStore := store.NewAgentStore(db, writer)
	handler := app.New(db)
	httpapi.RegisterAuth(handler, authService, cfg.PublicURL, cfg.SecureCookies)
	httpapi.RegisterProfiles(handler, authService, authService, cfg.PublicURL)
	httpapi.RegisterConversations(handler, authService, conversationService, pump, cfg.PublicURL)
	httpapi.RegisterMessages(handler, authService, messageService, pump, cfg.PublicURL)
	httpapi.RegisterRealtime(handler, authService, socket, cfg.PublicURL)
	httpapi.RegisterAgentWorker(handler, agentStore, pump)
	httpapi.RegisterCapabilities(handler, authService, agentStore)
	return &serverHandler{Handler: handler, pump: pump, hub: hub}, nil
}

func bootstrapAdmin(arguments []string, stdin *os.File, stdout io.Writer) error {
	flags := flag.NewFlagSet("bootstrap-admin", flag.ContinueOnError)
	statePath := flags.String("state-path", "", "required SQLite state path")
	username := flags.String("username", "", "administrator username")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if strings.TrimSpace(*statePath) == "" || strings.TrimSpace(*username) == "" {
		return fmt.Errorf("state-path and username are required")
	}
	password, err := readPassword(stdin, stdout)
	if err != nil {
		return err
	}
	db, err := store.Open(*statePath, 1)
	if err != nil {
		return fmt.Errorf("open persistence: %w", err)
	}
	defer db.Close()
	writer, err := store.NewWriter(db, 1)
	if err != nil {
		return fmt.Errorf("start persistence writer: %w", err)
	}
	defer writer.Close()
	service, err := auth.NewService(store.NewAuthStore(db, writer), time.Now, rand.Reader)
	if err != nil {
		return fmt.Errorf("start authentication: %w", err)
	}
	if err := service.Bootstrap(context.Background(), auth.Bootstrap{Username: *username, Password: password}); err != nil {
		return fmt.Errorf("bootstrap administrator: %w", err)
	}
	_, err = fmt.Fprintln(stdout, "administrator bootstrapped")
	return err
}

func readPassword(stdin *os.File, stdout io.Writer) (string, error) {
	if stdin == nil {
		return "", fmt.Errorf("password input is required")
	}
	if term.IsTerminal(int(stdin.Fd())) {
		if _, err := fmt.Fprint(stdout, "Password: "); err != nil {
			return "", err
		}
		password, err := term.ReadPassword(int(stdin.Fd()))
		_, _ = fmt.Fprintln(stdout)
		if err != nil {
			return "", fmt.Errorf("read password from terminal: %w", err)
		}
		return string(password), nil
	}
	reader := bufio.NewReader(io.LimitReader(stdin, 130))
	password, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("read password from stdin: %w", err)
	}
	if len(password) > 129 {
		return "", fmt.Errorf("password input is too long")
	}
	password = bytesTrimLineEnding(password)
	if strings.ContainsAny(string(password), "\r\n") {
		return "", fmt.Errorf("password input must contain exactly one line")
	}
	return string(password), nil
}

func bytesTrimLineEnding(value []byte) []byte {
	if len(value) > 0 && value[len(value)-1] == '\n' {
		value = value[:len(value)-1]
		if len(value) > 0 && value[len(value)-1] == '\r' {
			value = value[:len(value)-1]
		}
	}
	return value
}
