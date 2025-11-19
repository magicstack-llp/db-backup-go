package app

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/magicstack-llp/db-backup-go/data"
	"github.com/magicstack-llp/db-backup-go/domain"
)

// BackupUseCase orchestrates the backup process
type BackupUseCase struct {
	databaseGateway *data.DatabaseGateway
	storageGateway   *data.StorageGateway
}

// NewBackupUseCase creates a new BackupUseCase instance
func NewBackupUseCase(databaseGateway *data.DatabaseGateway, storageGateway *data.StorageGateway) *BackupUseCase {
	return &BackupUseCase{
		databaseGateway: databaseGateway,
		storageGateway:   storageGateway,
	}
}

// Execute executes the backup process
func (uc *BackupUseCase) Execute(retentionCount int, backupDir string, s3Bucket string, s3Path string, compress bool) error {
	databases, err := uc.databaseGateway.ListDatabases()
	if err != nil {
		return fmt.Errorf("failed to list databases: %w", err)
	}
	
	for _, db := range databases {
		timestamp := time.Now().Format("20060102150405")
		backupFilename := fmt.Sprintf("%s-%s.sql", db.Name, timestamp)
		
		if backupDir != "" {
			// Local backup
			dbBackupDir := filepath.Join(backupDir, db.Name)
			if err := os.MkdirAll(dbBackupDir, 0755); err != nil {
				return fmt.Errorf("failed to create backup directory: %w", err)
			}
			
			backupFilepath := filepath.Join(dbBackupDir, backupFilename)
			if err := uc.databaseGateway.BackupDatabase(db.Name, backupFilepath); err != nil {
				fmt.Printf("Error backing up database %s: %v\n", db.Name, err)
				continue
			}
			
			// Optional compression
			finalPath := backupFilepath
			if compress {
				gzPath := backupFilepath + ".gz"
				if err := compressFile(backupFilepath, gzPath); err != nil {
					fmt.Printf("Warning: failed to compress backup: %v\n", err)
				} else {
					os.Remove(backupFilepath)
					finalPath = gzPath
				}
			}
			
			if err := uc.storageGateway.StoreBackup(finalPath, db.Name, "", ""); err != nil {
				fmt.Printf("Error storing backup: %v\n", err)
			}
			
			if err := uc.storageGateway.CleanupBackups(db.Name, retentionCount, "", ""); err != nil {
				fmt.Printf("Error cleaning up backups: %v\n", err)
			}
		} else if s3Bucket != "" && s3Path != "" {
			// S3 backup
			localBackupPath := filepath.Join(os.TempDir(), backupFilename)
			if err := uc.databaseGateway.BackupDatabase(db.Name, localBackupPath); err != nil {
				fmt.Printf("Error backing up database %s: %v\n", db.Name, err)
				continue
			}
			
			finalLocalPath := localBackupPath
			finalKeyName := backupFilename
			
			if compress {
				gzLocal := localBackupPath + ".gz"
				if err := compressFile(localBackupPath, gzLocal); err != nil {
					fmt.Printf("Warning: failed to compress backup: %v\n", err)
				} else {
					os.Remove(localBackupPath)
					finalLocalPath = gzLocal
					finalKeyName = backupFilename + ".gz"
				}
			}
			
			s3Key := fmt.Sprintf("%s/%s/%s", s3Path, db.Name, finalKeyName)
			if err := uc.storageGateway.StoreBackup(finalLocalPath, db.Name, s3Bucket, s3Key); err != nil {
				fmt.Printf("Error storing backup to S3: %v\n", err)
			}
			
			if err := uc.storageGateway.CleanupBackups(db.Name, retentionCount, s3Bucket, s3Path); err != nil {
				fmt.Printf("Error cleaning up S3 backups: %v\n", err)
			}
			
			// Clean up temporary file
			os.Remove(finalLocalPath)
		}
	}
	
	return nil
}

// compressFile compresses a file using gzip
func compressFile(srcPath string, dstPath string) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()
	
	dst, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer dst.Close()
	
	gzWriter := gzip.NewWriter(dst)
	defer gzWriter.Close()
	
	_, err = io.Copy(gzWriter, src)
	return err
}

