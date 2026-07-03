package api

import (
	"fmt"
	"sync/atomic"
	"testing"

	"releasehub/backend/internal/config"
	"releasehub/backend/internal/database"

	"gorm.io/gorm"
)

// testDBSeq 为每个测试数据库分配全局唯一序号，保证共享内存 SQLite 的命名隔离。
var testDBSeq uint64

// newTestDB 打开一个相互隔离、全内存的测试数据库。
//
// API 包所有测试统一使用本函数创建数据库，解决历史遗留的两种不一致：
//
//  1. 部分 handler 测试内联 gorm.Open(sqlite.Open(":memory:"))：连接池开启多连接时，
//     每个 connection 会得到各自独立的内存库（:memory: 按 connection 隔离），
//     且只迁移了部分表，与走 database.Open + 全量 Migrate 的用例不一致。
//
//  2. repository/token 等用例使用 t.TempDir() 下的文件库：当测试触发后台 syncer 异步下载
//     （checkLatest 等会 EnqueueSyncRepository，worker 懒启动后不会随测试结束而 Stop），
//     测试返回后 t.TempDir() 清理会删除数据库所在目录，泄漏的 worker 仍继续写并需要在该目录
//     创建回滚 journal 文件，从而偶发 "attempt to write a readonly database"。
//
// 本辅助采用命名共享内存 SQLite（file:<unique>?mode=memory&cache=shared）+ 单连接池：
//   - 全内存，无 journal/wal 边车文件，彻底消除上述只读告警；
//   - SetMaxOpenConns(1) 规避共享内存库在多连接下各自拷贝的问题，并避免并发写锁竞争；
//   - 每次调用使用唯一数据库名，各测试完全隔离，支持 t.Parallel 与 -count=N 多次运行。
//
// 这里不注册 t.Cleanup 关闭连接：泄漏的后台 worker 仍持有同一 *gorm.DB 继续写，
// 关闭会使其报 "database is closed" 反而产生新的噪声；内存库随测试进程退出自动回收。
func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	seq := atomic.AddUint64(&testDBSeq, 1)
	dsn := fmt.Sprintf("file:releasehub_testdb_%d?mode=memory&cache=shared", seq)

	db, err := database.Open(config.DatabaseConfig{Driver: "sqlite", DSN: dsn})
	if err != nil {
		t.Fatalf("打开测试数据库失败: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("获取底层数据库连接失败: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	// 不设置 ConnMaxLifetime，保留默认 0（永不回收），避免单连接被回收后重建为空库。

	if err := database.Migrate(db); err != nil {
		t.Fatalf("迁移测试数据库失败: %v", err)
	}

	return db
}
