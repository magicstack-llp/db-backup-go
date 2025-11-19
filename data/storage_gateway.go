package data

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// StorageGateway handles backup storage operations
type StorageGateway struct {
	backupDir string
	s3Client  *s3.Client
	s3Bucket  string
	s3Path    string
}

// NewStorageGateway creates a new StorageGateway instance
func NewStorageGateway(backupDir string, s3Bucket string, s3Path string,
	awsAccessKeyID string, awsSecretAccessKey string) (*StorageGateway, error) {
	
	sg := &StorageGateway{
		backupDir: backupDir,
		s3Bucket:  s3Bucket,
		s3Path:    s3Path,
	}
	
	// Setup S3 client if bucket is provided
	if s3Bucket != "" {
		ctx := context.Background()
		cfg, err := config.LoadDefaultConfig(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to load AWS config: %w", err)
		}
		
		// Override credentials if provided
		if awsAccessKeyID != "" && awsSecretAccessKey != "" {
			cfg.Credentials = credentials.NewStaticCredentialsProvider(awsAccessKeyID, awsSecretAccessKey, "")
		}
		
		sg.s3Client = s3.NewFromConfig(cfg)
	}
	
	return sg, nil
}

// StoreBackup stores a backup file (local or S3)
func (sg *StorageGateway) StoreBackup(backupPath string, dbName string, s3Bucket string, s3Key string) error {
	if s3Bucket != "" && s3Key != "" {
		return sg.storeS3Backup(backupPath, s3Bucket, s3Key)
	}
	return sg.storeLocalBackup(backupPath)
}

// storeLocalBackup stores backup locally
func (sg *StorageGateway) storeLocalBackup(backupPath string) error {
	fmt.Printf("Successfully created local backup: %s\n", backupPath)
	return nil
}

// storeS3Backup uploads backup to S3
func (sg *StorageGateway) storeS3Backup(backupPath string, s3Bucket string, s3Key string) error {
	file, err := os.Open(backupPath)
	if err != nil {
		return fmt.Errorf("failed to open backup file: %w", err)
	}
	defer file.Close()
	
	ctx := context.Background()
	_, err = sg.s3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s3Bucket),
		Key:    aws.String(s3Key),
		Body:   file,
	})
	
	if err != nil {
		return fmt.Errorf("failed to upload to S3: %w", err)
	}
	
	fmt.Printf("Successfully uploaded %s to %s/%s\n", backupPath, s3Bucket, s3Key)
	return nil
}

// CleanupBackups removes old backups based on retention count
func (sg *StorageGateway) CleanupBackups(dbName string, retentionCount int, s3Bucket string, s3Path string) error {
	if s3Bucket != "" && s3Path != "" {
		return sg.cleanupS3Backups(dbName, retentionCount, s3Bucket, s3Path)
	}
	return sg.cleanupLocalBackups(dbName, retentionCount)
}

// cleanupLocalBackups removes old local backups
func (sg *StorageGateway) cleanupLocalBackups(dbName string, retentionCount int) error {
	dbBackupDir := filepath.Join(sg.backupDir, dbName)
	if _, err := os.Stat(dbBackupDir); os.IsNotExist(err) {
		return nil
	}
	
	entries, err := os.ReadDir(dbBackupDir)
	if err != nil {
		return fmt.Errorf("failed to read backup directory: %w", err)
	}
	
	// Filter backup files (.gz or .sql)
	var backups []os.FileInfo
	for _, entry := range entries {
		if !entry.IsDir() && (strings.HasSuffix(entry.Name(), ".gz") || strings.HasSuffix(entry.Name(), ".sql")) {
			info, err := entry.Info()
			if err != nil {
				continue
			}
			backups = append(backups, info)
		}
	}
	
	// Sort by modification time (newest first)
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].ModTime().After(backups[j].ModTime())
	})
	
	// Remove old backups
	if len(backups) > retentionCount {
		for _, oldBackup := range backups[retentionCount:] {
			backupPath := filepath.Join(dbBackupDir, oldBackup.Name())
			if err := os.Remove(backupPath); err != nil {
				fmt.Printf("Failed to remove old backup %s: %v\n", oldBackup.Name(), err)
			} else {
				fmt.Printf("Removed old local backup: %s\n", oldBackup.Name())
			}
		}
	}
	
	return nil
}

// cleanupS3Backups removes old S3 backups
func (sg *StorageGateway) cleanupS3Backups(dbName string, retentionCount int, s3Bucket string, s3Path string) error {
	prefix := fmt.Sprintf("%s/%s/", s3Path, dbName)
	ctx := context.Background()
	
	// List objects with prefix
	paginator := s3.NewListObjectsV2Paginator(sg.s3Client, &s3.ListObjectsV2Input{
		Bucket: aws.String(s3Bucket),
		Prefix:  aws.String(prefix),
	})
	
	var objects []types.Object
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("failed to list S3 objects: %w", err)
		}
		objects = append(objects, page.Contents...)
	}
	
	// Sort by LastModified (newest first)
	sort.Slice(objects, func(i, j int) bool {
		return objects[i].LastModified.After(*objects[j].LastModified)
	})
	
	// Remove old backups
	if len(objects) > retentionCount {
		for _, oldBackup := range objects[retentionCount:] {
			_, err := sg.s3Client.DeleteObject(ctx, &s3.DeleteObjectInput{
				Bucket: aws.String(s3Bucket),
				Key:    oldBackup.Key,
			})
			if err != nil {
				fmt.Printf("Failed to remove old S3 backup %s: %v\n", *oldBackup.Key, err)
			} else {
				fmt.Printf("Removed old S3 backup: %s\n", *oldBackup.Key)
			}
		}
	}
	
	return nil
}

