package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/Niksinikhilesh045/enterprise-workflow-platform/services/api/internal/httpapi"
	"github.com/Niksinikhilesh045/enterprise-workflow-platform/services/api/internal/store"
)

func main() {
	addr := os.Getenv("HTTP_ADDR")
	if addr == "" { addr = ":8080" }
	handler := httpapi.New(store.NewMemoryStore())
	srv := &http.Server{Addr: addr, Handler: handler, ReadHeaderTimeout: 5 * time.Second}
	log.Printf("enterprise workflow API listening on %s", addr)
	log.Fatal(srv.ListenAndServe())
}
