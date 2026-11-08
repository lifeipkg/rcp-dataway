package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lifeipkg/rcp-dataway/internal/config"
	"github.com/lifeipkg/rcp-dataway/internal/guard"
	"github.com/lifeipkg/rcp-dataway/internal/store"
	"github.com/lifeipkg/rcp-dataway/internal/web"
)

// 数据网关：业务服务访问数据库的唯一出口（RL-005）。
//
// 业务服务不持驱动、不写裸 SQL：只读查询必须通过守卫（单条 SELECT + 表白名单），
// 写入必须命中服务端登记的模板。连接与凭据只在本进程内。
func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("[dataway] 配置不可用: %v", err)
	}

	// 模板与白名单在建立连接之前校验：配置退化时不要先连上库再失败。
	templates, err := guard.NewTemplates(cfg.Templates)
	if err != nil {
		log.Fatalf("[dataway] 写入模板不合规: %v", err)
	}
	sqlGuard := guard.NewGuard(cfg.AllowedTables)
	log.Printf("[dataway] 白名单 %v，模板 %v", sqlGuard.AllowedTables(), templates.IDs())

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	db, err := store.Open(ctx, cfg.DSN)
	if err != nil {
		// 数据面不可用的网关没有任何意义，不启动比半可用好。
		log.Fatalf("[dataway] %v", err)
	}
	defer db.Close()

	handler := web.NewHandler(db, sqlGuard, templates, cfg.DatasourceName, cfg.DefaultMaxRows)
	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           web.NewRouter(handler),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		log.Printf("[dataway] listening on :%s, datasource=%s", cfg.Port, cfg.DatasourceName)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[dataway] 监听失败: %v", err)
		}
	}()

	<-stop
	// 优雅退出：正在执行的查询写完再关，避免调用方看到被截断的连接。
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("[dataway] 优雅退出超时: %v", err)
	}
	log.Print("[dataway] stopped")
}
