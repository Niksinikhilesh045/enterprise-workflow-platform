package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/Niksinikhilesh045/enterprise-workflow-platform/services/api/internal/auth"
)

func main() {
	var (
		subject = flag.String("subject", "local-user", "token subject")
		tenant  = flag.String("tenant", "acme", "tenant ID")
		role    = flag.String("role", string(auth.RoleAdmin), "role: admin|builder|requester|approver|auditor")
		ttl     = flag.Duration("ttl", time.Hour, "token lifetime")
	)
	flag.Parse()

	manager, err := auth.NewManager(os.Getenv("AUTH_SECRET"))
	if err != nil {
		log.Fatal(err)
	}
	token, err := manager.Issue(*subject, *tenant, auth.Role(*role), *ttl)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(token)
}
