package main

import (
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"
	"waddlemap/internal/network"
	"waddlemap/internal/storage"
	"waddlemap/internal/transaction"
	"waddlemap/internal/types"
)

func main() {
	// 0. Logging Setup
	logFile, err := os.OpenFile("server.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("Failed to open log file: %v", err)
	}
	defer logFile.Close()

	multiWriter := io.MultiWriter(os.Stdout, logFile)
	log.SetOutput(multiWriter)
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	log.Println("----------------------------------------")
	log.Println("WaddleMap Server Initializing...")

	// 1. Config
	cfg := &types.DBSchemaConfig{
		PayloadSize: 1024,
		DataPath:    "./waddlemap_db",
		SyncMode:    "strict",
	}

	// 2. Storage
	storageMgr, err := storage.NewVectorManager(cfg)
	if err != nil {
		log.Fatalf("Failed to init storage: %v", err)
	}
	defer storageMgr.Close()

	// 3. Transaction Manager
	txMgr := transaction.NewManager(storageMgr)
	txMgr.Start()

	// 4. Server
	server := network.NewServer(6969, txMgr)

	// Graceful Shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		if err := server.Start(); err != nil {
			log.Fatalf("Server error: %v", err)
		}
	}()

	log.Println("Server started. Press Ctrl+C to stop.")
	<-sigChan
	log.Println("Shutting down...")
}
