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
	token := flag.String("token", os.Getenv("GOSH_UI_TOKEN"), "optional bearer token required for browser UI access")
	workspaces := flag.String("workspaces", os.Getenv("GOSH_WORKSPACES_FILE"), "workspace metadata JSON file")
	flag.Parse()

	server, err := ui.NewServerWithOptions(ui.ServerOptions{
		AuthToken:     *token,
		WorkspacePath: *workspaces,
	})
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
