// Command threadhall runs the Threadhall server.
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/tuebingen-quant-society/threadhall/internal/app"
)

var version = "dev"

func main() {
	if len(os.Args) == 2 && os.Args[1] == "version" {
		fmt.Println(version)
		return
	}

	address := flag.String("addr", ":8080", "HTTP listen address")
	flag.Parse()

	server := &http.Server{Addr: *address, Handler: app.New()}
	log.Printf("Threadhall %s listening on %s", version, *address)
	log.Fatal(server.ListenAndServe())
}
