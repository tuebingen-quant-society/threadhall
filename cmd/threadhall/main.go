// Command threadhall runs the Threadhall server.
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/tuebingen-quant-society/threadhall/internal/app"
	"github.com/tuebingen-quant-society/threadhall/internal/config"
	store "github.com/tuebingen-quant-society/threadhall/internal/store/sqlite"
)

var version = "dev"

func main() {
	if len(os.Args) == 2 && os.Args[1] == "version" {
		fmt.Println(version)
		return
	}
	if len(os.Args) < 2 || os.Args[1] != "serve" {
		log.Fatal("usage: threadhall serve [options] | threadhall version")
	}
	if err := serve(os.Args[2:]); err != nil {
		log.Fatal(err)
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

	server := &http.Server{Addr: *address, Handler: app.New(db)}
	log.Printf("Threadhall %s listening on %s", version, *address)
	return server.ListenAndServe()
}
