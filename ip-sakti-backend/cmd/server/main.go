package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/heythisissud/ip-sakti-backend/internal/api"
	"github.com/heythisissud/ip-sakti-backend/internal/classifier"
	"github.com/heythisissud/ip-sakti-backend/internal/retriever"
	"github.com/heythisissud/ip-sakti-backend/internal/store"
	"github.com/heythisissud/ip-sakti-backend/pkg/config"
	qdrantpb "github.com/qdrant/go-client/qdrant"
	"google.golang.org/genai"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	// ── Logger ────────────────────────────────────────────────────────────
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// ── Config ────────────────────────────────────────────────────────────
	cfg, err := config.Load()
	if err != nil {
		logger.Error("failed to load config", "error", err)
		os.Exit(1)
	}
	logger.Info("config loaded", "env", cfg.Env, "port", cfg.Port)

	// ── Database pool ─────────────────────────────────────────────────────
	initCtx, initCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer initCancel()

	pool, err := store.NewPool(initCtx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	logger.Info("database connected")

	// ── Gemini client (used by classifier) ───────────────────────────────
	geminiClient, err := genai.NewClient(context.Background(), &genai.ClientConfig{
		APIKey:  cfg.GeminiAPIKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		logger.Error("failed to create Gemini client", "error", err)
		os.Exit(1)
	}
	logger.Info("Gemini client initialised")

	// ── Qdrant gRPC client ────────────────────────────────────────────────
	// Connection failure here is fatal: a retriever without Qdrant is useless.
	qdrantAddr := cfg.QdrantHost + ":" + cfg.QdrantGRPCPort
	qdrantConn, err := grpc.NewClient(qdrantAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		logger.Error("failed to connect to Qdrant", "addr", qdrantAddr, "error", err)
		os.Exit(1)
	}
	defer qdrantConn.Close()
	qdrantClient := qdrantpb.NewPointsClient(qdrantConn)
	logger.Info("Qdrant client initialised", "addr", qdrantAddr)

	// ── Embedder (text-embedding-004 via Google AI) ───────────────────────
	embedder, err := retriever.NewEmbedder(cfg.GeminiAPIKey, logger)
	if err != nil {
		logger.Error("failed to create embedder", "error", err)
		os.Exit(1)
	}
	logger.Info("embedder initialised")

	// ── Retriever ─────────────────────────────────────────────────────────
	// cfg.QdrantAPIKey is "" for local Qdrant; non-empty for Qdrant Cloud.
	ret := retriever.NewRetriever(qdrantClient, embedder, cfg.QdrantAPIKey, logger)

	// ── Dependency injection ──────────────────────────────────────────────
	st := store.NewStore(pool)
	cl := classifier.NewClassifier(geminiClient, logger)
	h := api.NewHandler(cfg, pool, st, cl, ret, logger)
	router := api.NewRouter(h, cfg, logger)

	// ── HTTP server ───────────────────────────────────────────────────────
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.Port),
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("server starting", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
	}()

	// ── Graceful shutdown ─────────────────────────────────────────────────
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serverErr:
		logger.Error("server error", "error", err)
		os.Exit(1)
	case sig := <-quit:
		logger.Info("shutdown signal received", "signal", sig.String())
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
		os.Exit(1)
	}
	logger.Info("server stopped gracefully")
}

