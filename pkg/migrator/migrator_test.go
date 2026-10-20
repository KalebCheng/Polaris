package migrator

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatalf("写入测试文件失败: %v", err)
	}
}

func TestLoadDirParsesUpDownAndLegacy(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "001_init.up.sql", "CREATE TABLE a();")
	writeFile(t, dir, "001_init.down.sql", "DROP TABLE a;")
	writeFile(t, dir, "002_legacy.sql", "ALTER TABLE a ADD b int;")
	writeFile(t, dir, "003_pair.up.sql", "SELECT 3;")
	writeFile(t, dir, "003_pair.down.sql", "SELECT -3;")
	writeFile(t, dir, "README.md", "忽略非迁移文件")

	migrations, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir 失败: %v", err)
	}
	if len(migrations) != 3 {
		t.Fatalf("迁移数量 = %d, 期望 3（README 应被忽略）", len(migrations))
	}
	if migrations[0].Version != 1 || migrations[0].Name != "init" {
		t.Fatalf("第一条迁移解析错误: %+v", migrations[0])
	}
	if migrations[0].Down == "" {
		t.Fatal("成对格式应解析出 down 内容")
	}
	if migrations[1].Version != 2 || migrations[1].Up == "" || migrations[1].Down != "" {
		t.Fatalf("遗留格式应仅解析为 up: %+v", migrations[1])
	}
	if migrations[2].Version != 3 || migrations[2].Up == "" || migrations[2].Down == "" {
		t.Fatalf("第三条迁移解析错误: %+v", migrations[2])
	}
}

func TestLoadDirMissingDirectory(t *testing.T) {
	if _, err := LoadDir(filepath.Join(t.TempDir(), "not-exist")); err == nil {
		t.Fatal("目录不存在应当报错")
	}
}
