package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"github.com/thorved/ssh-vpn/backend/internal/config"
	"github.com/thorved/ssh-vpn/backend/internal/tunnel"
)

func main() {
	_ = godotenv.Load(".env")
	_ = godotenv.Load("backend/.env")

	cfg := config.MustLoad()

	server, err := tunnel.NewServer(cfg)
	if err != nil {
		log.Fatalf("init ssh vpn: %v", err)
	}

	go func() {
		if err := server.Run(); err != nil {
			log.Fatalf("ssh vpn stopped: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- server.Shutdown()
	}()

	select {
	case err := <-done:
		if err != nil {
			log.Printf("ssh vpn shutdown error: %v", err)
		}
	case <-ctx.Done():
		log.Printf("ssh vpn shutdown timed out")
	}
}
