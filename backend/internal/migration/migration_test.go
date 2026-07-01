package migration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadFilesSkipsInitAndSortsByName(t *testing.T) {
	dir := t.TempDir()
	writeMigrationFile(t, dir, "20260701_b.sql", "SELECT 2;")
	writeMigrationFile(t, dir, "init.sql", "SELECT 0;")
	writeMigrationFile(t, dir, "20260701_a.sql", "SELECT 1;")
	writeMigrationFile(t, dir, "notes.txt", "ignore")

	files, err := LoadFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}
	if files[0].Name != "20260701_a.sql" || files[1].Name != "20260701_b.sql" {
		t.Fatalf("unexpected order: %+v", files)
	}
	if files[0].Checksum == "" || files[0].Checksum == files[1].Checksum {
		t.Fatalf("unexpected checksums: %+v", files)
	}
}

func TestSplitSQLStatementsKeepsSemicolonsInsideQuotedStrings(t *testing.T) {
	statements := SplitSQLStatements(`
SET @sql = 'SELECT ''a;b''';
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
`)
	if len(statements) != 4 {
		t.Fatalf("expected 4 statements, got %d: %#v", len(statements), statements)
	}
	if !strings.Contains(statements[0], "a;b") {
		t.Fatalf("expected semicolon inside quoted string to be preserved: %q", statements[0])
	}
}

func TestValidateAppliedChecksumRejectsMismatch(t *testing.T) {
	err := ValidateAppliedChecksum("20260701_demo.sql", "current", "recorded")
	if err == nil {
		t.Fatal("expected checksum mismatch error")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") || !strings.Contains(err.Error(), "20260701_demo.sql") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestShouldApplyMigrationSkipsAlreadyAppliedFile(t *testing.T) {
	file := File{Name: "20260701_demo.sql", Checksum: "same"}
	shouldApply, err := shouldApplyMigration(file, "same", true)
	if err != nil {
		t.Fatal(err)
	}
	if shouldApply {
		t.Fatal("expected already applied migration to be skipped")
	}
}

func writeMigrationFile(t *testing.T, dir string, name string, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
