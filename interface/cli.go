package interface

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/magicstack-llp/db-backup-go/app"
	"github.com/magicstack-llp/db-backup-go/data"
	"github.com/spf13/cobra"
)

var (
	configPath      string
	connectionName  string
	retention       int
	storageType     string
	backupDir       string
	mysqldumpPath   string
	compress        bool
	noCompress      bool
)

// defaultConfigPath returns the default path for .env file
func defaultConfigPath() string {
	xdg := os.Getenv("XDG_CONFIG_HOME")
	base := os.Getenv("HOME")
	if xdg != "" {
		base = xdg
	} else {
		base = filepath.Join(base, ".config")
	}
	return filepath.Join(base, "database-backup", ".env")
}

// ensureConfigFile ensures the config file exists
func ensureConfigFile(configPath string) error {
	if _, err := os.Stat(configPath); err == nil {
		return nil
	}
	
	fmt.Printf("Config not found at %s — let's create one.\n", configPath)
	cfgDir := filepath.Dir(configPath)
	if err := os.MkdirAll(cfgDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}
	
	return initConfigInteractive(configPath)
}

// promptString prompts for a string input
func promptString(prompt string, defaultValue string) string {
	fmt.Print(prompt)
	if defaultValue != "" {
		fmt.Printf(" [%s]: ", defaultValue)
	} else {
		fmt.Print(": ")
	}
	
	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	
	if input == "" {
		return defaultValue
	}
	return input
}

// promptInt prompts for an integer input
func promptInt(prompt string, defaultValue int) int {
	for {
		input := promptString(prompt, fmt.Sprintf("%d", defaultValue))
		if input == "" {
			return defaultValue
		}
		val, err := strconv.Atoi(input)
		if err == nil {
			return val
		}
		fmt.Println("Invalid input. Please enter a number.")
	}
}

// promptBool prompts for a yes/no input
func promptBool(prompt string, defaultValue bool) bool {
	defaultStr := "n"
	if defaultValue {
		defaultStr = "y"
	}
	
	for {
		input := strings.ToLower(promptString(prompt, defaultStr))
		if input == "" {
			return defaultValue
		}
		if input == "y" || input == "yes" {
			return true
		}
		if input == "n" || input == "no" {
			return false
		}
		fmt.Println("Please enter 'y' or 'n'.")
	}
}

// initConfigInteractive interactively creates or updates a .env config file
func initConfigInteractive(configPath string) error {
	existing := make(map[string]string)
	if _, err := os.Stat(configPath); err == nil {
		existing, _ = godotenv.Read(configPath)
		if !promptBool("Config exists. Do you want to overwrite it?", false) {
			fmt.Println("Aborted. Existing config left unchanged.")
			return nil
		}
	}
	
	fmt.Println("Setting up storage and global configuration...")
	fmt.Println("(Database connections are managed separately with 'add' command)")
	
	backupDriver := promptString("Backup driver (local/s3)", existing["BACKUP_DRIVER"])
	if backupDriver == "" {
		backupDriver = "local"
	}
	backupDriver = strings.ToLower(backupDriver)
	
	var backupDir, s3Bucket, s3Path, awsAccessKeyID, awsSecretAccessKey string
	
	if backupDriver == "local" {
		backupDir = promptString("Local backup directory", existing["BACKUP_DIR"])
	} else {
		s3Bucket = promptString("S3 bucket name", existing["S3_BUCKET"])
		s3Path = promptString("S3 base path", existing["S3_PATH"])
		if s3Path == "" {
			s3Path = "backups"
		}
		awsAccessKeyID = promptString("AWS Access Key ID", existing["AWS_ACCESS_KEY_ID"])
		fmt.Print("AWS Secret Access Key: ")
		reader := bufio.NewReader(os.Stdin)
		awsSecretAccessKey, _ = reader.ReadString('\n')
		awsSecretAccessKey = strings.TrimSpace(awsSecretAccessKey)
		if awsSecretAccessKey == "" {
			awsSecretAccessKey = existing["AWS_SECRET_ACCESS_KEY"]
		}
	}
	
	retentionDefault := 5
	if existing["RETENTION_COUNT"] != "" {
		if val, err := strconv.Atoi(existing["RETENTION_COUNT"]); err == nil {
			retentionDefault = val
		}
	}
	retentionCount := promptInt("Retention count (how many backups to keep)", retentionDefault)
	
	// Write .env file
	lines := []string{
		fmt.Sprintf("BACKUP_DRIVER=%s", backupDriver),
		fmt.Sprintf("RETENTION_COUNT=%d", retentionCount),
	}
	
	if backupDriver == "local" {
		lines = append(lines, fmt.Sprintf("BACKUP_DIR=%s", backupDir))
	} else {
		lines = append(lines,
			fmt.Sprintf("S3_BUCKET=%s", s3Bucket),
			fmt.Sprintf("S3_PATH=%s", s3Path),
			fmt.Sprintf("AWS_ACCESS_KEY_ID=%s", awsAccessKeyID),
			fmt.Sprintf("AWS_SECRET_ACCESS_KEY=%s", awsSecretAccessKey),
		)
	}
	
	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}
	
	fmt.Printf("Created config at %s\n", configPath)
	fmt.Println("Use 'db-backup add' to add database connections.")
	return nil
}

// resolveExecutable finds a way to run the CLI from cron
func resolveExecutable() string {
	exe, err := exec.LookPath("db-backup")
	if err == nil {
		return exe
	}
	
	// Try to find go binary
	goExe, err := exec.LookPath("go")
	if err == nil {
		return fmt.Sprintf("%s run .", goExe)
	}
	
	return "db-backup"
}

// setupCronInteractive sets up crontab interactively
func setupCronInteractive(configPath string) error {
	fmt.Println("Let's set up your cron schedule for db-backup.")
	
	if err := ensureConfigFile(configPath); err != nil {
		return err
	}
	
	// Check for connections
	connManager, err := data.NewConnectionManager("")
	if err != nil {
		return fmt.Errorf("failed to create connection manager: %w", err)
	}
	
	connections, err := connManager.ListConnections()
	if err != nil {
		return fmt.Errorf("failed to list connections: %w", err)
	}
	
	var selectedConnection string
	if len(connections) == 0 {
		fmt.Println("No connections found. Please add a connection first with 'db-backup add'")
		return nil
	} else if len(connections) == 1 {
		selectedConnection = connections[0]
		fmt.Printf("Using connection: %s\n", selectedConnection)
	} else {
		fmt.Println("Available connections:")
		for i, conn := range connections {
			fmt.Printf("  %d. %s\n", i+1, conn)
		}
		choice := promptInt("Select connection for cron", 1)
		if choice < 1 || choice > len(connections) {
			fmt.Println("Invalid selection.")
			return nil
		}
		selectedConnection = connections[choice-1]
	}
	
	storageChoice := promptString("Storage to use (local/s3/config)", "config")
	storageChoice = strings.ToLower(storageChoice)
	
	defaultSchedule := "0 3,15 * * *"
	scheduleInput := promptString("Enter a cron expression (5 fields) or times (24h HH:MM) comma-separated", defaultSchedule)
	if scheduleInput == "" {
		scheduleInput = defaultSchedule
	}
	
	// Parse schedule
	var cronLines []string
	exe := resolveExecutable()
	storageFlag := ""
	if storageChoice == "local" || storageChoice == "s3" {
		storageFlag = fmt.Sprintf(" --%s", storageChoice)
	}
	cmd := fmt.Sprintf("%s backup --config \"%s\" --connection %s%s", exe, configPath, selectedConnection, storageFlag)
	
	// Check if it's a cron expression (5 fields)
	parts := strings.Fields(scheduleInput)
	if len(parts) == 5 {
		cronLines = append(cronLines, fmt.Sprintf("%s %s", scheduleInput, cmd))
	} else {
		// Try to parse as time list
		times := strings.Split(scheduleInput, ",")
		for _, t := range times {
			t = strings.TrimSpace(t)
			if matched, _ := regexp.MatchString(`^\d{2}:\d{2}$`, t); matched {
				parts := strings.Split(t, ":")
				hour, _ := strconv.Atoi(parts[0])
				minute, _ := strconv.Atoi(parts[1])
				if hour >= 0 && hour <= 23 && minute >= 0 && minute <= 59 {
					cronLines = append(cronLines, fmt.Sprintf("%d %d * * * %s", minute, hour, cmd))
				}
			}
		}
		if len(cronLines) == 0 {
			cronLines = append(cronLines, fmt.Sprintf("%s %s", defaultSchedule, cmd))
		}
	}
	
	fmt.Println("\nCron entries to be installed:")
	for _, line := range cronLines {
		fmt.Printf("  %s\n", line)
	}
	fmt.Println()
	
	if err := installCrontab(cronLines); err != nil {
		return fmt.Errorf("failed to install crontab: %w", err)
	}
	
	fmt.Println("✓ Cron entries installed successfully!")
	return nil
}

// installCrontab installs or updates user's crontab
func installCrontab(lines []string) error {
	// Read existing crontab
	cmd := exec.Command("crontab", "-l")
	existingBytes, _ := cmd.Output()
	existing := string(existingBytes)
	
	// Remove old db-backup managed block
	re := regexp.MustCompile(`(?s)# BEGIN db-backup.*?# END db-backup\s*`)
	existing = re.ReplaceAllString(existing, "")
	
	// Append new lines
	newCron := existing
	if strings.TrimSpace(newCron) != "" {
		newCron += "\n"
	}
	newCron += strings.Join(lines, "\n") + "\n"
	
	// Install new crontab
	installCmd := exec.Command("crontab", "-")
	installCmd.Stdin = strings.NewReader(newCron)
	if err := installCmd.Run(); err != nil {
		return fmt.Errorf("failed to install crontab: %w", err)
	}
	
	return nil
}

// backupCmd handles the backup command
func backupCmd(cmd *cobra.Command, args []string) error {
	// Resolve config path
	if configPath == "" {
		configPath = os.Getenv("DATABASE_BACKUP_CONFIG")
		if configPath == "" {
			configPath = defaultConfigPath()
		}
	}
	
	if err := ensureConfigFile(configPath); err != nil {
		return err
	}
	
	// Load .env file
	if err := godotenv.Load(configPath); err != nil {
		fmt.Printf("Warning: failed to load .env file: %v\n", err)
	}
	
	// Load connection
	connManager, err := data.NewConnectionManager("")
	if err != nil {
		return fmt.Errorf("failed to create connection manager: %w", err)
	}
	
	if connectionName == "" {
		connections, err := connManager.ListConnections()
		if err != nil {
			return fmt.Errorf("failed to list connections: %w", err)
		}
		
		if len(connections) == 0 {
			return fmt.Errorf("no connections found. Use 'db-backup add' to add a connection")
		} else if len(connections) == 1 {
			connectionName = connections[0]
			fmt.Printf("Using connection: %s\n", connectionName)
		} else {
			fmt.Println("Available connections:")
			for i, conn := range connections {
				fmt.Printf("  %d. %s\n", i+1, conn)
			}
			choice := promptInt("Select connection", 1)
			if choice < 1 || choice > len(connections) {
				return fmt.Errorf("invalid selection")
			}
			connectionName = connections[choice-1]
		}
	}
	
	conn, err := connManager.GetConnection(connectionName)
	if err != nil {
		return fmt.Errorf("connection '%s' not found: %w", connectionName, err)
	}
	
	// Determine storage type
	if storageType == "" {
		localFlag, _ := cmd.Flags().GetBool("local")
		s3Flag, _ := cmd.Flags().GetBool("s3")
		
		if localFlag {
			storageType = "local"
		} else if s3Flag {
			storageType = "s3"
		} else if conn.StorageDriver != "" {
			storageType = strings.ToLower(conn.StorageDriver)
		} else {
			storageType = strings.ToLower(os.Getenv("BACKUP_DRIVER"))
		}
	}
	
	// Determine retention count
	retentionCount := retention
	if retentionCount == 0 {
		if val := os.Getenv("RETENTION_COUNT"); val != "" {
			if parsed, err := strconv.Atoi(val); err == nil {
				retentionCount = parsed
			}
		}
		if retentionCount == 0 {
			retentionCount = 5
		}
	}
	
	// Determine compression
	shouldCompress := compress
	if noCompress {
		shouldCompress = false
	}
	if !compress && !noCompress {
		shouldCompress = true // default
	}
	
	// Create database gateway
	dbGateway := data.NewDatabaseGateway(
		conn.Host, conn.Port, conn.User, conn.Password,
		conn.MysqldumpPath, conn.ExcludedDBs,
		conn.SSHHost, conn.SSHPort, conn.SSHUser, conn.SSHKeyPath,
		conn.BastionHost, conn.BastionPort, conn.BastionUser, conn.BastionKeyPath,
	)
	defer dbGateway.Close()
	
	// Create storage gateway
	var storageGateway *data.StorageGateway
	var effectiveBackupDir, effectiveS3Bucket, effectiveS3Path string
	
	if storageType == "local" {
		effectiveBackupDir = backupDir
		if effectiveBackupDir == "" {
			effectiveBackupDir = conn.Path
		}
		if effectiveBackupDir == "" {
			effectiveBackupDir = os.Getenv("BACKUP_DIR")
		}
		if effectiveBackupDir == "" {
			return fmt.Errorf("please specify --backup-dir, set path in connection, or set BACKUP_DIR in .env")
		}
		
		storageGateway, err = data.NewStorageGateway(effectiveBackupDir, "", "", "", "")
		if err != nil {
			return fmt.Errorf("failed to create storage gateway: %w", err)
		}
	} else if storageType == "s3" {
		effectiveS3Bucket = conn.S3Bucket
		if effectiveS3Bucket == "" {
			effectiveS3Bucket = os.Getenv("S3_BUCKET")
		}
		if effectiveS3Bucket == "" {
			return fmt.Errorf("please set s3_bucket in connection, set S3_BUCKET in .env, or use --s3 with proper configuration")
		}
		
		effectiveS3Path = conn.Path
		if effectiveS3Path == "" {
			effectiveS3Path = os.Getenv("S3_PATH")
		}
		if effectiveS3Path == "" {
			effectiveS3Path = "backups"
		}
		
		awsAccessKeyID := os.Getenv("AWS_ACCESS_KEY_ID")
		awsSecretAccessKey := os.Getenv("AWS_SECRET_ACCESS_KEY")
		
		storageGateway, err = data.NewStorageGateway("", effectiveS3Bucket, effectiveS3Path, awsAccessKeyID, awsSecretAccessKey)
		if err != nil {
			return fmt.Errorf("failed to create storage gateway: %w", err)
		}
	} else {
		return fmt.Errorf("please specify a storage type: --local or --s3, set storage_driver in connection, or set BACKUP_DRIVER in .env")
	}
	
	// Create use case and execute
	useCase := app.NewBackupUseCase(dbGateway, storageGateway)
	return useCase.Execute(retentionCount, effectiveBackupDir, effectiveS3Bucket, effectiveS3Path, shouldCompress)
}

// addCmd handles the add command
func addCmd(cmd *cobra.Command, args []string) error {
	connManager, err := data.NewConnectionManager("")
	if err != nil {
		return fmt.Errorf("failed to create connection manager: %w", err)
	}
	
	// Get connection name
	name := connectionName
	if name == "" {
		name = promptString("Connection name", "")
		if name == "" {
			return fmt.Errorf("connection name is required")
		}
	}
	
	// Check if connection exists
	existing, _ := connManager.GetConnection(name)
	if existing != nil {
		if !promptBool(fmt.Sprintf("Connection '%s' already exists. Overwrite?", name), false) {
			fmt.Println("Aborted.")
			return nil
		}
	}
	
	// Get connection details
	host := promptString("MySQL host", getHost(existing))
	if host == "" {
		host = "localhost"
	}
	
	port := promptInt("MySQL port", getPort(existing))
	if port == 0 {
		port = 3306
	}
	
	user := promptString("MySQL user", getUser(existing))
	if user == "" {
		user = "root"
	}
	
	fmt.Print("MySQL password: ")
	reader := bufio.NewReader(os.Stdin)
	password, _ := reader.ReadString('\n')
	password = strings.TrimSpace(password)
	if password == "" && existing != nil {
		password = existing.Password
	}
	
	mysqldumpPath := promptString("mysqldump path", existing.MysqldumpPath)
	if mysqldumpPath == "" {
		if path, err := exec.LookPath("mysqldump"); err == nil {
			mysqldumpPath = path
		} else {
			mysqldumpPath = "/opt/homebrew/opt/mysql-client/bin/mysqldump"
		}
		if !promptBool(fmt.Sprintf("Use mysqldump at '%s'?", mysqldumpPath), true) {
			mysqldumpPath = promptString("mysqldump path", mysqldumpPath)
		}
	}
	
	excludedStr := promptString("Comma-separated list of databases to exclude (besides system DBs)", "")
	var excludedDBs []string
	if excludedStr != "" {
		for _, db := range strings.Split(excludedStr, ",") {
			if db = strings.TrimSpace(db); db != "" {
				excludedDBs = append(excludedDBs, db)
			}
		}
	}
	
	// Storage settings
	storageDriver := promptString("Storage driver (local/s3, leave empty to use .env)", existing.StorageDriver)
	storageDriver = strings.ToLower(storageDriver)
	if storageDriver == "" {
		storageDriver = existing.StorageDriver
	}
	
	var path, s3Bucket string
	if storageDriver == "local" {
		path = promptString("Backup directory path", existing.Path)
	} else if storageDriver == "s3" {
		s3Bucket = promptString("S3 bucket name", existing.S3Bucket)
		path = promptString("S3 path prefix", existing.Path)
	}
	
	// SSH settings
	sshHost := promptString("SSH host (leave empty if not using SSH)", existing.SSHHost)
	var sshPort int
	var sshUser, sshKeyPath string
	var bastionHost, bastionUser, bastionKeyPath string
	var bastionPort int
	
	if sshHost != "" {
		sshPort = promptInt("SSH port", existing.SSHPort)
		if sshPort == 0 {
			sshPort = 22
		}
		sshUser = promptString("SSH user", existing.SSHUser)
		sshKeyPath = promptString("SSH key path", existing.SSHKeyPath)
		
		bastionHost = promptString("Bastion host (leave empty if not using bastion)", existing.BastionHost)
		if bastionHost != "" {
			bastionPort = promptInt("Bastion port", existing.BastionPort)
			if bastionPort == 0 {
				bastionPort = 22
			}
			bastionUser = promptString("Bastion user", existing.BastionUser)
			if bastionUser == "" {
				bastionUser = sshUser
			}
			bastionKeyPath = promptString("Bastion key path", existing.BastionKeyPath)
			if bastionKeyPath == "" {
				bastionKeyPath = sshKeyPath
			}
		}
	} else if promptBool("Do you want to configure SSH tunnel for this connection?", false) {
		sshHost = promptString("SSH host", "")
		sshPort = promptInt("SSH port", 22)
		sshUser = promptString("SSH user", "")
		sshKeyPath = promptString("SSH key path", "")
		
		if promptBool("Use bastion host (double-hop SSH)?", false) {
			bastionHost = promptString("Bastion host", "")
			bastionPort = promptInt("Bastion port", 22)
			bastionUser = promptString("Bastion SSH user", sshUser)
			bastionKeyPath = promptString("Bastion SSH key path", sshKeyPath)
		}
	}
	
	// Create connection
	newConn := &data.Connection{
		Host:           host,
		Port:           port,
		User:           user,
		Password:       password,
		MysqldumpPath:  mysqldumpPath,
		ExcludedDBs:    excludedDBs,
		StorageDriver:  storageDriver,
		Path:           path,
		S3Bucket:       s3Bucket,
		SSHHost:        sshHost,
		SSHPort:        sshPort,
		SSHUser:        sshUser,
		SSHKeyPath:     sshKeyPath,
		BastionHost:    bastionHost,
		BastionPort:    bastionPort,
		BastionUser:    bastionUser,
		BastionKeyPath: bastionKeyPath,
	}
	
	if existing != nil {
		if err := connManager.UpdateConnection(name, newConn); err != nil {
			return fmt.Errorf("failed to update connection: %w", err)
		}
		fmt.Printf("Connection '%s' updated successfully.\n", name)
	} else {
		if err := connManager.AddConnection(name, newConn); err != nil {
			return fmt.Errorf("failed to add connection: %w", err)
		}
		fmt.Printf("Connection '%s' added successfully.\n", name)
	}
	
	return nil
}

func getHost(conn *data.Connection) string {
	if conn == nil {
		return ""
	}
	return conn.Host
}

func getPort(conn *data.Connection) int {
	if conn == nil {
		return 3306
	}
	return conn.Port
}

func getUser(conn *data.Connection) string {
	if conn == nil {
		return ""
	}
	return conn.User
}

// removeCmd handles the remove command
func removeCmd(cmd *cobra.Command, args []string) error {
	connManager, err := data.NewConnectionManager("")
	if err != nil {
		return fmt.Errorf("failed to create connection manager: %w", err)
	}
	
	name := connectionName
	if name == "" {
		name = promptString("Connection name", "")
		if name == "" {
			return fmt.Errorf("connection name is required")
		}
	}
	
	if _, err := connManager.GetConnection(name); err != nil {
		return fmt.Errorf("connection '%s' not found", name)
	}
	
	if !promptBool(fmt.Sprintf("Are you sure you want to remove connection '%s'?", name), false) {
		fmt.Println("Aborted.")
		return nil
	}
	
	if err := connManager.RemoveConnection(name); err != nil {
		return fmt.Errorf("failed to remove connection: %w", err)
	}
	
	fmt.Printf("Connection '%s' removed successfully.\n", name)
	return nil
}

// listCmd handles the list command
func listCmd(cmd *cobra.Command, args []string) error {
	connManager, err := data.NewConnectionManager("")
	if err != nil {
		return fmt.Errorf("failed to create connection manager: %w", err)
	}
	
	connections, err := connManager.ListConnections()
	if err != nil {
		return fmt.Errorf("failed to list connections: %w", err)
	}
	
	if len(connections) == 0 {
		fmt.Println("No connections found. Use 'db-backup add' to add a connection.")
		return nil
	}
	
	fmt.Println("Available connections:")
	for _, connName := range connections {
		conn, err := connManager.GetConnection(connName)
		if err != nil {
			continue
		}
		
		storageInfo := ""
		if conn.StorageDriver != "" {
			storageInfo = fmt.Sprintf(" [storage: %s", conn.StorageDriver)
			if conn.Path != "" {
				storageInfo += fmt.Sprintf(", path: %s", conn.Path)
			}
			if conn.StorageDriver == "s3" && conn.S3Bucket != "" {
				storageInfo += fmt.Sprintf(", bucket: %s", conn.S3Bucket)
			}
			storageInfo += "]"
		}
		
		fmt.Printf("  %s: %s@%s:%d%s\n", connName, conn.User, conn.Host, conn.Port, storageInfo)
	}
	
	return nil
}

// initCmd handles the init command
func initCmd(cmd *cobra.Command, args []string) error {
	if configPath == "" {
		configPath = os.Getenv("DATABASE_BACKUP_CONFIG")
		if configPath == "" {
			configPath = defaultConfigPath()
		}
	}
	return initConfigInteractive(configPath)
}

// cronCmd handles the cron command
func cronCmd(cmd *cobra.Command, args []string) error {
	if configPath == "" {
		configPath = os.Getenv("DATABASE_BACKUP_CONFIG")
		if configPath == "" {
			configPath = defaultConfigPath()
		}
	}
	return setupCronInteractive(configPath)
}

// NewRootCmd creates the root command
func NewRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "db-backup",
		Short: "Database backup tool with multiple connection support",
		Long:  "A command-line tool for backing up MySQL databases to local storage or AWS S3.",
	}
	
	// Backup command
	backupCmd := &cobra.Command{
		Use:   "backup",
		Short: "Run backup for a database connection",
		RunE:  backupCmd,
	}
	backupCmd.Flags().StringVar(&configPath, "config", "", "Path to the .env file")
	backupCmd.Flags().StringVar(&connectionName, "connection", "", "Name of the connection to use for backup")
	backupCmd.Flags().IntVar(&retention, "retention", 0, "Number of backups to retain")
	backupCmd.Flags().Bool("local", false, "Store backups locally")
	backupCmd.Flags().Bool("s3", false, "Store backups in S3")
	backupCmd.Flags().StringVar(&backupDir, "backup-dir", "", "Local directory to store backups in")
	backupCmd.Flags().StringVar(&mysqldumpPath, "mysqldump", "", "Path to mysqldump binary")
	backupCmd.Flags().BoolVar(&compress, "compress", true, "Compress backups with gzip")
	backupCmd.Flags().BoolVar(&noCompress, "no-compress", false, "Don't compress backups")
	
	// Add command
	addCmd := &cobra.Command{
		Use:   "add",
		Short: "Add a new database connection",
		RunE:  addCmd,
	}
	addCmd.Flags().StringVar(&connectionName, "name", "", "Name for this connection")
	
	// Remove command
	removeCmd := &cobra.Command{
		Use:   "remove",
		Short: "Remove a database connection",
		RunE:  removeCmd,
	}
	removeCmd.Flags().StringVar(&connectionName, "name", "", "Name of the connection to remove")
	
	// List command
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List all database connections",
		RunE:  listCmd,
	}
	
	// Init command
	initCmd := &cobra.Command{
		Use:   "init",
		Short: "Interactively create/update the config file",
		RunE:  initCmd,
	}
	initCmd.Flags().StringVar(&configPath, "config", "", "Path to the .env file")
	
	// Cron command
	cronCmd := &cobra.Command{
		Use:   "cron",
		Short: "Interactively set up crontab",
		RunE:  cronCmd,
	}
	cronCmd.Flags().StringVar(&configPath, "config", "", "Path to the .env file")
	
	rootCmd.AddCommand(backupCmd, addCmd, removeCmd, listCmd, initCmd, cronCmd)
	
	return rootCmd
}

