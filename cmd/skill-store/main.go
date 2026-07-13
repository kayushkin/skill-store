// Command skill-store runs the HTTP registry for Claude Code / Agent skills.
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	skillstore "github.com/kayushkin/skill-store"
)

func main() {
	addr := os.Getenv("SKILL_STORE_ADDR")
	if addr == "" {
		addr = ":8301"
	}
	dataDir := os.Getenv("SKILL_STORE_DATA_DIR")

	store, err := skillstore.Open(dataDir)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer store.Close()

	// Where installed skills are copied. Defaults to ~/.claude/skills; an
	// override is what lets a test drive the install/uninstall/seed routes
	// without writing into the live skills directory.
	if installDir := os.Getenv("SKILL_STORE_INSTALL_DIR"); installDir != "" {
		store.SetInstallDir(installDir)
	}

	mux := http.NewServeMux()
	skillstore.RegisterHandlers(mux, store)

	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("skill-store listening on %s (data=%s install=%s)", addr, store.DataDir(), store.InstallDir())
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Println("shutting down…")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}
