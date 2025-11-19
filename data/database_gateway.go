package data

import (
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	_ "github.com/go-sql-driver/mysql"
	"github.com/magicstack-llp/db-backup-go/domain"
)

// DatabaseGateway handles database operations
type DatabaseGateway struct {
	host            string
	port            int
	user            string
	password        string
	mysqldumpPath   string
	excludedDBs     map[string]bool
	sshTunnel       *SSHTunnel
	effectiveHost   string
	effectivePort   int
}

// NewDatabaseGateway creates a new DatabaseGateway instance
func NewDatabaseGateway(host string, port int, user string, password string,
	mysqldumpPath string, excludedDBs []string,
	sshHost string, sshPort int, sshUser string, sshKeyPath string,
	bastionHost string, bastionPort int, bastionUser string, bastionKeyPath string) *DatabaseGateway {
	
	// Default excluded databases
	systemExcluded := map[string]bool{
		"information_schema": true,
		"performance_schema": true,
		"mysql":              true,
		"sys":                true,
	}
	
	// Add user-specified exclusions
	for _, db := range excludedDBs {
		if db != "" {
			systemExcluded[strings.TrimSpace(db)] = true
		}
	}
	
	// Resolve mysqldump path
	if mysqldumpPath == "" {
		mysqldumpPath = os.Getenv("MYSQLDUMP_PATH")
	}
	if mysqldumpPath == "" {
		mysqldumpPath = "mysqldump"
	}
	
	gateway := &DatabaseGateway{
		host:          host,
		port:          port,
		user:          user,
		password:      password,
		mysqldumpPath: mysqldumpPath,
		excludedDBs:   systemExcluded,
		effectiveHost: host,
		effectivePort: port,
	}
	
	// Setup SSH tunnel if configured
	if sshHost != "" && sshUser != "" && sshKeyPath != "" {
		gateway.sshTunnel = NewSSHTunnel(
			sshHost, sshPort, sshUser, sshKeyPath,
			host, port,
			bastionHost, bastionPort, bastionUser, bastionKeyPath,
		)
	}
	
	return gateway
}

// ensureSSHTunnel ensures SSH tunnel is established if configured
func (dg *DatabaseGateway) ensureSSHTunnel() error {
	if dg.sshTunnel != nil {
		if dg.effectiveHost == dg.host && dg.effectivePort == dg.port {
			// Tunnel not started yet
			localPort, err := dg.sshTunnel.Start()
			if err != nil {
				return fmt.Errorf("failed to start SSH tunnel: %w", err)
			}
			dg.effectiveHost = "127.0.0.1"
			dg.effectivePort = localPort
		}
	} else {
		dg.effectiveHost = dg.host
		dg.effectivePort = dg.port
	}
	return nil
}

// cleanupSSHTunnel cleans up SSH tunnel if it exists
func (dg *DatabaseGateway) cleanupSSHTunnel() {
	if dg.sshTunnel != nil {
		dg.sshTunnel.Stop()
		dg.effectiveHost = dg.host
		dg.effectivePort = dg.port
	}
}

// ListDatabases lists all databases excluding system databases
func (dg *DatabaseGateway) ListDatabases() ([]*domain.Database, error) {
	if err := dg.ensureSSHTunnel(); err != nil {
		return nil, err
	}
	
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/", dg.user, dg.password, dg.effectiveHost, dg.effectivePort)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MySQL: %w", err)
	}
	defer db.Close()
	
	rows, err := db.Query("SHOW DATABASES")
	if err != nil {
		return nil, fmt.Errorf("failed to query databases: %w", err)
	}
	defer rows.Close()
	
	var databases []*domain.Database
	for rows.Next() {
		var dbName string
		if err := rows.Scan(&dbName); err != nil {
			continue
		}
		
		if !dg.excludedDBs[dbName] {
			databases = append(databases, domain.NewDatabase(dbName))
		}
	}
	
	return databases, nil
}

// BackupDatabase backs up a database using mysqldump
func (dg *DatabaseGateway) BackupDatabase(dbName string, backupPath string) error {
	if err := dg.ensureSSHTunnel(); err != nil {
		return err
	}
	
	// Resolve mysqldump absolute path
	mysqldump := dg.mysqldumpPath
	if !filepath.IsAbs(mysqldump) {
		resolved, err := exec.LookPath(mysqldump)
		if err != nil {
			return fmt.Errorf("mysqldump not found. Set MYSQLDUMP_PATH in .env or ensure '%s' is in PATH", mysqldump)
		}
		mysqldump = resolved
	}
	
	// Ensure backup directory exists
	backupDir := filepath.Dir(backupPath)
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return fmt.Errorf("failed to create backup directory: %w", err)
	}
	
	// Build mysqldump command
	cmd := exec.Command(mysqldump,
		fmt.Sprintf("--host=%s", dg.effectiveHost),
		fmt.Sprintf("--port=%d", dg.effectivePort),
		fmt.Sprintf("--user=%s", dg.user),
		fmt.Sprintf("--password=%s", dg.password),
		"--single-transaction",
		"--quick",
		"--skip-lock-tables",
		dbName,
	)
	
	// Create output file
	outFile, err := os.Create(backupPath)
	if err != nil {
		return fmt.Errorf("failed to create backup file: %w", err)
	}
	defer outFile.Close()
	
	cmd.Stdout = outFile
	cmd.Stderr = os.Stderr
	
	// Run mysqldump
	if err := cmd.Run(); err != nil {
		// Clean up empty file
		if info, statErr := os.Stat(backupPath); statErr == nil && info.Size() == 0 {
			os.Remove(backupPath)
		}
		return fmt.Errorf("mysqldump failed: %w", err)
	}
	
	// Verify file exists and is non-empty
	info, err := os.Stat(backupPath)
	if err != nil {
		return fmt.Errorf("backup file not found: %w", err)
	}
	if info.Size() == 0 {
		os.Remove(backupPath)
		return fmt.Errorf("backup file is empty. Check mysqldump permissions and options")
	}
	
	return nil
}

// Close closes SSH tunnel and cleanup resources
func (dg *DatabaseGateway) Close() {
	dg.cleanupSSHTunnel()
}

