package main

import (
	"flag"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"
	"waddlemap/internal/logger"
	"waddlemap/internal/network"
	"waddlemap/internal/storage"
	"waddlemap/internal/transaction"
	"waddlemap/internal/types"
)

func main() {
	// Flags
	port := flag.Int("port", 6969, "Port to listen on")
	quiet := flag.Bool("quiet", false, "Disable info logging (log only errors)")
	flag.Parse()

	// 0. Logging Setup
	logFile, err := os.OpenFile("server.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("Failed to open log file: %v", err)
	}
	defer logFile.Close()

	// 0. Logging Setup
	multiWriter := io.MultiWriter(os.Stdout, logFile)
	logger.Setup(multiWriter)

	if *quiet {
		logger.SetLevel(logger.LevelError)
	} else {
		// Default Info
		logger.SetLevel(logger.LevelInfo)
	}

	logger.Info("----------------------------------------")
	logger.Info("WaddleMap Server Initializing...")

	// 1. Config
	cfg := &types.DBSchemaConfig{
		PayloadSize: 1024,
		DataPath:    "./waddlemap_db",
		SyncMode:    "strict",
	}

	// 2. Storage
	storageMgr, err := storage.NewVectorManager(cfg)
	if err != nil {
		logger.Fatal("Failed to init storage: %v", err)
	}
	defer storageMgr.Close()

	// 3. Transaction Manager
	txMgr := transaction.NewManager(storageMgr)
	txMgr.Start()

	// 4. Server
	server := network.NewServer(*port, txMgr)

	// Graceful Shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		if err := server.Start(); err != nil {
			logger.Fatal("Server error: %v", err)
		}
	}()

	logger.Info("Server started on port %d. Press Ctrl+C to stop.", *port)
	<-sigChan
	logger.Info("Shutting down...")
}
