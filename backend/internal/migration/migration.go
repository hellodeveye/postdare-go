package migration

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type File struct {
	Name     string
	Path     string
	SQL      string
	Checksum string
}

func Run(ctx context.Context, database *sql.DB, dir string, out io.Writer) error {
	if strings.TrimSpace(dir) == "" {
		dir = "./migrations"
	}
	conn, err := database.Conn(ctx)
	if err != nil {
		return fmt.Errorf("open migration connection: %w", err)
	}
	defer conn.Close()

	if err := ensureSchemaMigrations(ctx, conn); err != nil {
		return err
	}
	files, err := LoadFiles(dir)
	if err != nil {
		return err
	}
	for _, file := range files {
		appliedChecksum, applied, err := appliedMigration(ctx, conn, file.Name)
		if err != nil {
			return err
		}
		shouldApply, err := shouldApplyMigration(file, appliedChecksum, applied)
		if err != nil {
			return err
		}
		if !shouldApply {
			writeLine(out, "skipped %s", file.Name)
			continue
		}
		if err := executeFile(ctx, conn, file); err != nil {
			return err
		}
		writeLine(out, "applied %s", file.Name)
	}
	return nil
}

func LoadFiles(dir string) ([]File, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read migration dir: %w", err)
	}
	var files []File
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if name == "init.sql" || !strings.HasSuffix(name, ".sql") {
			continue
		}
		path := filepath.Join(dir, name)
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read migration %s: %w", name, err)
		}
		files = append(files, File{
			Name:     name,
			Path:     path,
			SQL:      string(raw),
			Checksum: Checksum(raw),
		})
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].Name < files[j].Name
	})
	return files, nil
}

func Checksum(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func ValidateAppliedChecksum(filename string, expected string, actual string) error {
	if expected == actual {
		return nil
	}
	return fmt.Errorf("migration checksum mismatch for %s: recorded %s, current %s", filename, actual, expected)
}

func shouldApplyMigration(file File, appliedChecksum string, applied bool) (bool, error) {
	if !applied {
		return true, nil
	}
	if err := ValidateAppliedChecksum(file.Name, file.Checksum, appliedChecksum); err != nil {
		return false, err
	}
	return false, nil
}

func ensureSchemaMigrations(ctx context.Context, conn *sql.Conn) error {
	_, err := conn.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS schema_migrations (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  filename VARCHAR(255) NOT NULL UNIQUE,
  checksum VARCHAR(64) NOT NULL,
  executed_at DATETIME NOT NULL
)`)
	if err != nil {
		return fmt.Errorf("ensure schema_migrations: %w", err)
	}
	return nil
}

func appliedMigration(ctx context.Context, conn *sql.Conn, filename string) (string, bool, error) {
	var checksum string
	err := conn.QueryRowContext(ctx, `SELECT checksum FROM schema_migrations WHERE filename = ?`, filename).Scan(&checksum)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("read schema_migrations for %s: %w", filename, err)
	}
	return checksum, true, nil
}

func executeFile(ctx context.Context, conn *sql.Conn, file File) error {
	statements := SplitSQLStatements(file.SQL)
	for i, statement := range statements {
		if _, err := conn.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("execute %s statement %d: %w", file.Name, i+1, err)
		}
	}
	if _, err := conn.ExecContext(
		ctx,
		`INSERT INTO schema_migrations (filename, checksum, executed_at) VALUES (?, ?, ?)`,
		file.Name,
		file.Checksum,
		time.Now(),
	); err != nil {
		return fmt.Errorf("record migration %s: %w", file.Name, err)
	}
	return nil
}

func SplitSQLStatements(sqlText string) []string {
	var statements []string
	var b strings.Builder
	var quote rune
	escaped := false
	for _, r := range sqlText {
		if quote != 0 {
			b.WriteRune(r)
			if escaped {
				escaped = false
				continue
			}
			if r == '\\' {
				escaped = true
				continue
			}
			if r == quote {
				quote = 0
			}
			continue
		}
		switch r {
		case '\'', '"', '`':
			quote = r
			b.WriteRune(r)
		case ';':
			statement := strings.TrimSpace(b.String())
			if statement != "" {
				statements = append(statements, statement)
			}
			b.Reset()
		default:
			b.WriteRune(r)
		}
	}
	statement := strings.TrimSpace(b.String())
	if statement != "" {
		statements = append(statements, statement)
	}
	return statements
}

func writeLine(out io.Writer, format string, args ...interface{}) {
	if out == nil {
		return
	}
	fmt.Fprintf(out, format+"\n", args...)
}
