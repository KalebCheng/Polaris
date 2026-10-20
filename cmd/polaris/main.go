// Polaris —— 统一身份认证与访问管理平台（IAM / IdP）。
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"

	"polaris/internal/app"
	"polaris/internal/auth"
	"polaris/internal/config"
	"polaris/internal/logger"
	"polaris/internal/repository"
	"polaris/internal/repository/postgres"
	"polaris/pkg/dbclient"
	"polaris/pkg/migrator"
)

func main() {
	migrateUp := flag.Bool("migrate", false, "执行数据库迁移（up）后退出")
	migrateDown := flag.Int("migrate-down", 0, "回滚最近 N 步迁移后退出")
	rotateProviderSecrets := flag.Bool("rotate-provider-secrets", false,
		"把提供商表里的明文密钥就地加密（迁移 027 后的存量回填；幂等，可重复执行）")
	flag.Parse()

	if *migrateUp || *migrateDown > 0 {
		os.Exit(runMigrationCLI(*migrateDown))
	}
	if *rotateProviderSecrets {
		os.Exit(runProviderSecretRotation())
	}

	application, err := app.New()
	if err != nil {
		logger.Fatal("failed to create application", "error", err)
	}
	if err := application.Start(); err != nil {
		logger.Fatal("failed to start application", "error", err)
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), app.DefaultShutdownTimeout)
	defer cancel()
	if err := application.Stop(ctx); err != nil {
		logger.Error("server forced to shutdown", "error", err)
	}
	logger.Info("server exited")
}

// runProviderSecretRotation 把 providers 表的明文密钥就地加密（幂等）。
//
// 背景：迁移 027 只改结构；存量行（第四阶段之前写入）的 client_secret 仍是明文。
// 读取侧对"解密失败"按明文兼容，因此本命令可在任意时刻执行、可重复执行：
// 已加密的行会被跳过（能解出来即视为已加密）。
func runProviderSecretRotation() int {
	configPath := os.Getenv(config.EnvConfigPath)
	if configPath == "" {
		configPath = "configs/app.yaml"
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "加载配置失败: %v\n", err)
		return 1
	}
	if cfg.Security.CredentialEncKey == "" {
		fmt.Fprintln(os.Stderr, "缺少 security.credential_enc_key（或 POLARIS_CREDENTIAL_ENC_KEY），无法加密提供商密钥")
		return 1
	}
	cipher, err := auth.NewSecretCipher(auth.DeriveKey(cfg.Security.CredentialEncKey))
	if err != nil {
		fmt.Fprintf(os.Stderr, "初始化密钥器失败: %v\n", err)
		return 1
	}
	ctx := context.Background()
	db, err := dbclient.New(dbclient.Config{
		DSN:          cfg.Database.DSN(),
		MaxOpenConns: int32(cfg.Database.MaxOpenConns),
		MaxIdleConns: int32(cfg.Database.MaxIdleConns),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "连接数据库失败: %v\n", err)
		return 1
	}
	defer db.Close()

	repo := postgres.NewProviderRepository(db)
	items, err := repo.List(ctx, repository.ListQuery{Limit: 10000})
	if err != nil {
		fmt.Fprintf(os.Stderr, "读取提供商失败: %v\n", err)
		return 1
	}
	rotated, skipped := 0, 0
	for _, item := range items {
		if item.ClientSecret == "" {
			skipped++
			continue
		}
		if _, err := cipher.Decrypt(item.ClientSecret); err == nil {
			skipped++
			continue
		}
		encrypted, err := cipher.Encrypt(item.ClientSecret)
		if err != nil {
			fmt.Fprintf(os.Stderr, "加密失败 provider=%s: %v\n", item.Name, err)
			return 1
		}
		item.ClientSecret = encrypted
		if err := repo.Update(ctx, item); err != nil {
			fmt.Fprintf(os.Stderr, "写回失败 provider=%s: %v\n", item.Name, err)
			return 1
		}
		rotated++
	}
	fmt.Printf("提供商密钥回填完成：已加密 %d 条，跳过 %d 条\n", rotated, skipped)
	return 0
}

// runMigrationCLI 独立执行迁移后退出（无需启动完整应用）
func runMigrationCLI(down int) int {
	configPath := os.Getenv(config.EnvConfigPath)
	if configPath == "" {
		configPath = "configs/app.yaml"
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "加载配置失败: %v\n", err)
		return 1
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, cfg.Database.DSN())
	if err != nil {
		fmt.Fprintf(os.Stderr, "连接数据库失败: %v\n", err)
		return 1
	}
	defer pool.Close()

	if down > 0 {
		rolled, err := migrator.Down(ctx, pool, cfg.Database.MigrationsDir, down)
		if err != nil {
			fmt.Fprintf(os.Stderr, "回滚失败: %v\n", err)
			return 1
		}
		fmt.Printf("已回滚迁移: %v\n", rolled)
		return 0
	}

	applied, err := migrator.Up(ctx, pool, cfg.Database.MigrationsDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "迁移失败: %v\n", err)
		return 1
	}
	fmt.Printf("已应用迁移: %v\n", applied)
	return 0
}
