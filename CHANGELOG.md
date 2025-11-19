# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Fixed
- Fixed package name issue: renamed `interface` package to `cli` to avoid Go reserved keyword conflict
- Fixed potential panic in SSH tunnel path expansion by adding length check before string slice access
- Removed unused imports across multiple files

### Changed
- Updated build process to output binaries to `build/` directory
- Added `.gitignore` file for better version control
- Added `CHANGELOG.md` for tracking project changes

## [0.1.0] - 2024-11-19

### Added
- Initial release of db-backup-go
- Support for multiple database connections
- Local and S3 storage backends
- SSH tunnel support (simple and bastion host)
- Gzip compression support
- Automatic backup cleanup based on retention policy
- Interactive CLI for configuration and connection management
- Cron setup functionality
- Command-line interface with Cobra
- Clean architecture implementation

### Features
- MySQL database backup using mysqldump
- Exclude system databases automatically
- Timestamped backups
- Separate folder for each database
- Configuration via .env file and connections.json
- Cross-platform support

