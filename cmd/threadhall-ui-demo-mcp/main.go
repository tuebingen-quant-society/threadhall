package main

import (
	"log"
	"os"

	"github.com/tuebingen-quant-society/threadhall/internal/demomcp"
)

func main() {
	if err := demomcp.Serve(os.Stdin, os.Stdout); err != nil {
		log.Fatal(err)
	}
}
