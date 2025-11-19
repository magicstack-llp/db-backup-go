# Database Backup Tool (Go)

A command-line tool for backing up MySQL databases to local storage or AWS S3, written in Go.

This is a Go implementation of the [database-backup](https://github.com/magicstack-llp/db-backup) Python tool with the same features and functionality.

## Features

- **Multiple database connections**: Manage multiple database connections with separate JSON storage.
- Back up all MySQL databases, excluding system databases.
- Store backups in a local directory or an AWS S3 bucket.
- Create a separate folder for each database.
- Timestamped backups for easy identification.
- Automatic cleanup of old backups based on a retention policy.
- Configuration via `.env` file (storage/global settings) and `connections.json` (database connections).
- Command-line interface for easy operation.
- Cron setup for automatic backups.
- SSH tunnel support (simple and bastion host).
- Gzip compression support.

## Requirements

- Go 1.21 or later
- MySQL client tools (provides `mysqldump`)

    On macOS (Homebrew):
    ```bash
    brew install mysql-client
    # Typical binary path: /opt/homebrew/opt/mysql-client/bin/mysqldump (Apple Silicon)
    ```

    On Debian/Ubuntu:
    ```bash
    sudo apt-get update
    sudo apt-get install mysql-client
    # Typical binary path: /usr/bin/mysqldump
    ```

    On Red Hat/CentOS/Fedora:
    ```bash
    sudo dnf install mysql
    # Typical binary path: /usr/bin/mysqldump
    ```

## Installation

### From Source

```bash
git clone https://github.com/magicstack-llp/db-backup.git
cd db-backup/db-backup-go
go build -o db-backup
sudo mv db-backup /usr/local/bin/
```

### Build Standalone Binary

```bash
cd db-backup-go
go build -o db-backup
```

The binary will be created in the current directory and can be copied anywhere.

## Quick Start

1. **Initialize storage configuration**

```bash
# Interactive init (sets up storage/global settings)
db-backup init
```

2. **Add a database connection**

```bash
# Add your first database connection
db-backup add --name production --host 127.0.0.1 --user root
```

3. **Run backup**

```bash
db-backup backup --connection production --local   # store on filesystem
db-backup backup --connection production --s3        # store on S3
```

## Configuration

The tool uses two separate configuration files:

1. **`.env` file**: Stores storage and global settings (S3 credentials, backup directory, retention count, etc.)
   - Default location: `~/.config/database-backup/.env` (or `${XDG_CONFIG_HOME}/database-backup/.env`)
   - Override with `--config` or `DATABASE_BACKUP_CONFIG` env var

2. **`connections.json` file**: Stores database connection details (host, port, user, password, etc.)
   - Default location: `~/.config/database-backup/connections.json`
   - Managed via CLI commands: `add`, `remove`, `list`

### Storage Configuration (.env)

Example `.env` (storage/global settings only):

```env
BACKUP_DRIVER=local  # local, s3
BACKUP_DIR=/Users/<USER>/backups/databases
RETENTION_COUNT=5
S3_BUCKET=mybucket
S3_PATH=backups
AWS_ACCESS_KEY_ID=XXXXXXX
AWS_SECRET_ACCESS_KEY=YYYYYYY
```

### Connection Management

Database connections are stored separately in JSON format. Use these commands:

- `db-backup add`: Add a new database connection
- `db-backup remove`: Remove a database connection
- `db-backup list`: List all database connections

Example `connections.json`:

```json
{
  "production": {
    "host": "127.0.0.1",
    "port": 3306,
    "user": "root",
    "password": "password",
    "mysqldump_path": "/opt/homebrew/opt/mysql-client/bin/mysqldump",
    "excluded_databases": ["db_1", "db_2"],
    "storage_driver": "local",
    "path": "/backups/production"
  },
  "staging": {
    "host": "192.168.1.100",
    "port": 3306,
    "user": "backup_user",
    "password": "secure_password",
    "mysqldump_path": "/usr/bin/mysqldump",
    "excluded_databases": [],
    "storage_driver": "s3",
    "s3_bucket": "my-backup-bucket",
    "path": "staging"
  },
  "remote_ssh": {
    "host": "127.0.0.1",
    "port": 3306,
    "user": "root",
    "password": "password",
    "ssh_host": "db.example.com",
    "ssh_port": 22,
    "ssh_user": "backup_user",
    "ssh_key_path": "/home/user/.ssh/id_rsa",
    "storage_driver": "local",
    "path": "/backups/remote"
  },
  "bastion_ssh": {
    "host": "127.0.0.1",
    "port": 3306,
    "user": "root",
    "password": "password",
    "ssh_host": "internal-db.example.com",
    "ssh_port": 22,
    "ssh_user": "backup_user",
    "ssh_key_path": "/home/user/.ssh/id_rsa",
    "bastion_host": "bastion.example.com",
    "bastion_port": 22,
    "bastion_user": "bastion_user",
    "bastion_key_path": "/home/user/.ssh/bastion_key",
    "storage_driver": "s3",
    "s3_bucket": "my-backup-bucket",
    "path": "bastion"
  }
}
```

## Usage

### Basic backup

```bash
# Backup using a specific connection
db-backup backup --connection production --local

# If only one connection exists, it will be used automatically
db-backup backup --local

# If multiple connections exist, you'll be prompted to select one
db-backup backup --local
```

### Backup options

- `--connection NAME`: Specify which connection to use (required if multiple connections exist)
- `--local`: Store backups locally
- `--s3`: Store backups in S3
- `--retention N`: Number of backups to retain (overrides .env)
- `--backup-dir PATH`: Local backup directory (overrides .env)
- `--mysqldump PATH`: Path to mysqldump binary (overrides connection setting)
- `--compress/--no-compress`: Compress backups with gzip (default: compress)
- `--config FILE`: Override .env config file path

### Examples

```bash
# Local backup with specific connection
db-backup backup --connection production --local

# S3 backup
db-backup backup --connection staging --s3

# Override retention count
db-backup backup --connection production --local --retention 10

# Custom backup directory
db-backup backup --connection production --local --backup-dir /custom/path
```

## Architecture

The database backup tool is built using a Clean Architecture approach, which separates the code into four layers:

- **Domain**: Contains the core business logic and entities of the application.
- **Data**: Contains the data access layer, which is responsible for interacting with the database and storage.
- **App**: Contains the application logic, which orchestrates the backup process.
- **Interface**: Contains the user interface, which is responsible for handling user input and displaying output.

This separation of concerns makes the application more modular, testable, and maintainable.

## SSH Tunnel Support

The tool supports connecting to MySQL databases through SSH tunnels, including:

### Simple SSH Tunnel

For databases accessible via a single SSH hop:

```bash
db-backup add --name remote --host 127.0.0.1 --port 3306 --user root --password pass \
  --ssh-host db.example.com --ssh-user backup_user --ssh-key-path ~/.ssh/id_rsa
```

### SSH with Bastion Host

For databases requiring a double-hop SSH connection (through a bastion host):

```bash
db-backup add --name bastion --host 127.0.0.1 --port 3306 --user root --password pass \
  --ssh-host internal-db.example.com --ssh-user backup_user --ssh-key-path ~/.ssh/id_rsa \
  --bastion-host bastion.example.com --bastion-user bastion_user --bastion-key-path ~/.ssh/bastion_key
```

### SSH Key Requirements

- SSH keys must be in a format supported by golang.org/x/crypto/ssh (RSA, ECDSA, Ed25519)
- Key files should have appropriate permissions (typically `600`)
- The SSH user must have access to the MySQL server on the remote host

## Cron Setup

You can set up cron jobs interactively:

```bash
db-backup cron
```

- You can enter either:
  - A full cron expression (5 fields), e.g. `0 3,15 * * *`
  - Or a comma-separated list of 24h times, e.g. `03:00,15:00`
- Default schedule: `0 3,15 * * *` (daily at 03:00 and 15:00)
- You'll be prompted to select a connection and storage type
- The CLI writes a managed block to your user crontab.

## Building

### Build for Current Platform

```bash
go build -o db-backup
```

### Cross-Platform Builds

**Linux:**
```bash
GOOS=linux GOARCH=amd64 go build -o db-backup-linux-amd64
```

**macOS:**
```bash
GOOS=darwin GOARCH=amd64 go build -o db-backup-darwin-amd64
GOOS=darwin GOARCH=arm64 go build -o db-backup-darwin-arm64
```

**Windows:**
```bash
GOOS=windows GOARCH=amd64 go build -o db-backup-windows-amd64.exe
```

## Configuration Reference

### .env File (Storage/Global Settings)

All storage and global settings are read from your .env file unless overridden by CLI flags.

- **BACKUP_DRIVER**: Where to store backups. One of: `local`, `s3`
- **BACKUP_DIR**: Base directory for local backups (used when BACKUP_DRIVER=local or with --local)
- **S3_BUCKET**: S3 bucket name (used when BACKUP_DRIVER=s3 or with --s3)
- **S3_PATH**: Prefix/path inside the bucket to store backups
- **AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY**: AWS credentials to access the bucket
- **RETENTION_COUNT**: Number of most recent backups to keep per database (default: 5)
- **DATABASE_BACKUP_CONFIG**: Optional env var to point the CLI to a different .env file

### connections.json (Database Connections)

Each connection includes:

- **host**: MySQL server host
- **port**: MySQL server port (default: 3306)
- **user**: MySQL username
- **password**: Password for the MySQL user
- **mysqldump_path**: Full path or command name to mysqldump (optional)
- **excluded_databases**: List of additional databases to skip (optional)
- **storage_driver**: Preferred storage driver for this connection (optional: `local` or `s3`)
- **path**: Storage path - backup directory for local storage or S3 path prefix (optional)
- **s3_bucket**: Preferred S3 bucket for this connection (optional)
- **ssh_host**: SSH hostname for tunnel (optional)
- **ssh_port**: SSH port (default: 22)
- **ssh_user**: SSH username for tunnel (optional)
- **ssh_key_path**: Path to SSH private key file (optional)
- **bastion_host**: Bastion host for double-hop SSH (optional)
- **bastion_port**: Bastion SSH port (default: 22)
- **bastion_user**: Bastion SSH username (optional, uses ssh_user if not provided)
- **bastion_key_path**: Bastion SSH key path (optional, uses ssh_key_path if not provided)

## Differences from Python Version

This Go implementation maintains feature parity with the Python version, with the following differences:

- **Performance**: Generally faster execution and lower memory footprint
- **Single Binary**: Can be compiled to a single standalone executable
- **No Python Dependencies**: No need for Python runtime or pip packages
- **Cross-Platform**: Easy cross-compilation for different platforms

## Contributing

Contributions are welcome! Please feel free to submit a pull request or open an issue if you have any suggestions or feedback.

## License

This project is licensed under the MIT License.

