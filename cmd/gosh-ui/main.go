package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"

	"go-shell/internal/ui"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8090", "HTTP address for the Go Shell UI")
	flag.Parse()

	server, err := ui.NewServer()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stdout, "Go Shell UI listening on http://%s\n", *addr)
	if err := http.ListenAndServe(*addr, server.Handler()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
