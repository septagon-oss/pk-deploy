// Implements: REQ-INFRA-006.
// Per: ADR-0029.
// Discipline: C-14.

package main

import (
	"log"
	"net/http"
	"time"

	"github.com/septagon-oss/pk-deploy/internal/controlplane"
)

func main() {
	cfg, err := controlplane.ConfigFromEnv()
	if err != nil {
		log.Fatal(err)
	}
	server, err := controlplane.NewServer(cfg, controlplane.NewStore(time.Now().UTC()))
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("pk-deploy control plane listening on %s", cfg.BindAddress)
	log.Fatal(http.ListenAndServe(cfg.BindAddress, server.Handler()))
}
