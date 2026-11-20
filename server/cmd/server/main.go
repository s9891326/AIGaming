// Command server 啟動遊戲後端：HTTP API 與（之後的）WebSocket 遊戲連線。
//
// 用法：
//
//	server               啟動伺服器
//	server -healthcheck  對本機 /api/healthz 發請求，成功回傳 0（給 docker HEALTHCHECK 用）
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata" // 內嵌時區資料，distroless 映像檔裡也能用 TZ=Asia/Taipei

	"aigaming/server/internal/config"
	"aigaming/server/internal/httpserver"
	"aigaming/server/internal/session"
	"aigaming/server/internal/ws"
	"aigaming/server/pkg/logger"
)

func main() {
	healthcheck := flag.Bool("healthcheck", false, "probe the local /api/healthz endpoint and exit")
	flag.Parse()

	cfg := config.Load()

	if *healthcheck {
		os.Exit(probe(cfg))
	}

	log := logger.New(logger.Options{
		Level:     cfg.LogLevel,
		Format:    cfg.LogFormat,
		Color:     cfg.LogColor,
		AddSource: cfg.LogLevel <= slog.LevelDebug,
	})
	slog.SetDefault(log)

	if err := run(cfg, log); err != nil {
		log.Error("server exited with error", "err", err)
		os.Exit(1)
	}
}

func run(cfg config.Config, log *slog.Logger) error {
	wsServer := ws.NewServer(log, session.NewHandler(log, cfg.Version))

	srv := &http.Server{
		Addr:              cfg.Addr(),
		Handler:           httpserver.NewRouter(cfg, log, wsServer),
		ReadHeaderTimeout: 5 * time.Second,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelWarn),
	}

	// 收到 Ctrl+C / SIGTERM 時優雅關閉，給進行中的請求最多 5 秒完成。
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		log.Info("server listening", "addr", cfg.Addr(), "mode", cfg.GinMode, "version", cfg.Version)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	log.Info("shutting down", "connections", wsServer.Count())
	// http.Server.Shutdown 不會處理已升級的 WebSocket 連線，要先自己關掉。
	wsServer.Close()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}
	log.Info("server stopped")
	return nil
}

// probe 對本機健康檢查端點發請求。distroless 映像檔沒有 curl，所以由程式自己做。
func probe(cfg config.Config) int {
	client := http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://127.0.0.1" + cfg.Addr() + "/api/healthz")
	if err != nil {
		fmt.Fprintln(os.Stderr, "healthcheck failed:", err)
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintln(os.Stderr, "healthcheck failed: status", resp.StatusCode)
		return 1
	}
	return 0
}
