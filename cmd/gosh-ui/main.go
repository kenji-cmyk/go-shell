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
	tlsCert := flag.String("tls-cert", os.Getenv("GOSH_UI_TLS_CERT"), "TLS certificate file for HTTPS")
	tlsKey := flag.String("tls-key", os.Getenv("GOSH_UI_TLS_KEY"), "TLS private key file for HTTPS")
	flag.Parse()

	server, err := ui.NewServerWithOptions(ui.ServerOptions{
		AuthToken:     *token,
		WorkspacePath: *workspaces,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if (*tlsCert == "") != (*tlsKey == "") {
		fmt.Fprintln(os.Stderr, "both -tls-cert and -tls-key are required for HTTPS")
		os.Exit(1)
	}

	scheme := "http"
	listen := func() error {
		return http.ListenAndServe(*addr, server.Handler())
	}
	if *tlsCert != "" {
		scheme = "https"
		listen = func() error {
			return http.ListenAndServeTLS(*addr, *tlsCert, *tlsKey, server.Handler())
		}
	}

	fmt.Fprintf(os.Stdout, "Go Shell UI listening on %s://%s\n", scheme, *addr)
	if err := listen(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
