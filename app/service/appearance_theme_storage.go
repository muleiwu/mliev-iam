package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type themeObjectStore interface {
	MaxFileBytes() int64
	Read(context.Context, string) ([]byte, string, error)
	Put(context.Context, []byte, string) (string, error)
	Delete(context.Context, string) error
}
type themeS3Store struct {
	storage *StorageService
	db      *gorm.DB
}

func newThemeObjectStore(ctx context.Context, db *gorm.DB) (themeObjectStore, error) {
	storage, err := NewStorageService(ctx)
	if err != nil {
		return nil, err
	}
	return &themeS3Store{storage: storage, db: db}, nil
}
func (s *themeS3Store) MaxFileBytes() int64 {
	size := s.storage.config.MaxFileSize
	if size <= 0 {
		size = 5
	}
	if size > 64 {
		size = 64
	}
	return size << 20
}
func (s *themeS3Store) Read(ctx context.Context, id string) ([]byte, string, error) {
	attachment, err := dao.NewAttachmentDao(s.db).GetByUUID(id)
	if err != nil {
		return nil, "", err
	}
	if attachment.FileSize > s.MaxFileBytes() {
		return nil, "", packageError("站内图片超过文件限制")
	}
	object, err := s.storage.s3Client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(s.storage.config.Bucket), Key: aws.String(attachment.StorageKey)})
	if err != nil {
		return nil, "", fmt.Errorf("读取站内图片失败: %w", err)
	}
	defer object.Body.Close()
	data, err := io.ReadAll(io.LimitReader(object.Body, s.MaxFileBytes()+1))
	if err != nil {
		return nil, "", err
	}
	if int64(len(data)) > s.MaxFileBytes() {
		return nil, "", packageError("站内图片超过文件限制")
	}
	if err = validateThemeImage(data, attachment.MimeType); err != nil {
		return nil, "", err
	}
	return data, attachment.MimeType, nil
}
func (s *themeS3Store) Put(ctx context.Context, data []byte, mime string) (string, error) {
	if int64(len(data)) > s.MaxFileBytes() {
		return "", packageError("图片超过存储设置的文件大小限制")
	}
	key := time.Now().Format("2006/01/02/") + uuid.NewString() + extensionFromImageMimeType(mime)
	if prefix := strings.Trim(s.storage.config.UploadPath, "/"); prefix != "" {
		key = prefix + "/" + key
	}
	input := &s3.PutObjectInput{Bucket: aws.String(s.storage.config.Bucket), Key: aws.String(key), Body: bytes.NewReader(data), ContentType: aws.String(mime)}
	if s.storage.config.PublicRead {
		input.ACL = types.ObjectCannedACLPublicRead
	}
	_, err := s.storage.s3Client.PutObject(ctx, input)
	// Returning the key even on an ambiguous write failure allows cleanup.
	return key, err
}
func (s *themeS3Store) Delete(ctx context.Context, key string) error {
	_, err := s.storage.s3Client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(s.storage.config.Bucket), Key: aws.String(key)})
	return err
}
