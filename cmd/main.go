package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/ilgooz/service-webman/service"
)

func main() {
	srv, err := service.New(
		service.WebhookOption("/webhook", ":4000"),
	)
	if err != nil {
		log.Fatal(err)
	}

	go func() {
		if err := srv.Start(); err != nil {
			log.Fatal(err)
		}
	}()

	abort := make(chan os.Signal, 1)
	signal.Notify(abort, syscall.SIGINT, syscall.SIGTERM)
	<-abort

	if err := srv.Close(); err != nil {
		log.Fatal(err)
	}
	log.Println("\ngracefully stopped")
}
