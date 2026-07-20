package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/open/go-web/pkg/helper"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/google/uuid"
)

// 头像上传的 MIME 白名单（SVG 用于 DiceBear 预设头像）。
var avatarAllowedMimeTypes = []string{
	"image/jpeg",
	"image/png",
	"image/webp",
	"image/svg+xml",
}

// 应用图标会在管理端裁剪为 PNG；接口同时兼容常见的安全栅格格式。
var applicationIconAllowedMimeTypes = []string{
	"image/jpeg",
	"image/png",
	"image/webp",
}

const presignedURLExpiry = 15 * time.Minute

// StorageUploadResult S3 上传结果
type StorageUploadResult struct {
	StorageKey string
	FileSize   int64
	MimeType   string
}

// StorageService S3 兼容对象存储服务。配置来自 DB settings（管理端存储设置页）。
type StorageService struct {
	s3Client *s3.Client
	config   *dto.StorageSettings
}

// NewStorageService 创建对象存储服务；未启用或必填项缺失时返回错误。
func NewStorageService(ctx context.Context) (*StorageService, error) {
	settings, err := NewSettingService(helper.GetDatabase()).GetStorageSettings()
	if err != nil {
		return nil, fmt.Errorf("读取对象存储配置失败: %w", err)
	}

	if !settings.Enabled {
		return nil, errors.New("对象存储未配置")
	}
	if settings.Bucket == "" || settings.AccessKey == "" || settings.SecretKey == "" {
		return nil, errors.New("对象存储配置不完整（bucket/accessKey/secretKey 必填）")
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(settings.Region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(settings.AccessKey, settings.SecretKey, "")),
	)
	if err != nil {
		return nil, fmt.Errorf("加载存储配置失败: %w", err)
	}

	var s3Client *s3.Client
	if settings.Endpoint != "" {
		// 自定义 endpoint（MinIO、白山云等）使用 path-style
		s3Client = s3.NewFromConfig(awsCfg, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(settings.Endpoint)
			o.UsePathStyle = true
		})
	} else {
		s3Client = s3.NewFromConfig(awsCfg)
	}

	return &StorageService{s3Client: s3Client, config: settings}, nil
}

// UploadAvatarFile 上传头像文件到对象存储，返回存储 key 与元信息。
func (s *StorageService) UploadAvatarFile(ctx context.Context, file *multipart.FileHeader) (*StorageUploadResult, error) {
	return s.uploadImageFile(ctx, file, avatarAllowedMimeTypes)
}

// UploadApplicationIconFile 上传应用图标文件到对象存储。
func (s *StorageService) UploadApplicationIconFile(ctx context.Context, file *multipart.FileHeader) (*StorageUploadResult, error) {
	return s.uploadImageFile(ctx, file, applicationIconAllowedMimeTypes)
}

func (s *StorageService) uploadImageFile(ctx context.Context, file *multipart.FileHeader, allowedMimeTypes []string) (*StorageUploadResult, error) {
	maxSizeMB := s.config.MaxFileSize
	if maxSizeMB <= 0 {
		maxSizeMB = 5
	}
	if file.Size > maxSizeMB*1024*1024 {
		return nil, fmt.Errorf("文件大小超过限制（最大 %d MB）", maxSizeMB)
	}

	src, err := file.Open()
	if err != nil {
		return nil, errors.New("无法打开文件")
	}
	defer src.Close()

	// 检测 MIME 类型（前 512 字节嗅探）
	buffer := make([]byte, 512)
	n, err := src.Read(buffer)
	if err != nil && err != io.EOF {
		return nil, errors.New("无法读取文件")
	}
	mimeType := http.DetectContentType(buffer[:n])

	// SVG 特殊处理：http.DetectContentType 会将 SVG 识别为 text/xml
	ext := strings.ToLower(filepath.Ext(file.Filename))
	if ext == ".svg" && (strings.HasPrefix(mimeType, "text/xml") || strings.HasPrefix(mimeType, "text/plain") || strings.HasPrefix(mimeType, "application/xml")) {
		mimeType = "image/svg+xml"
	}

	allowed := false
	for _, t := range allowedMimeTypes {
		if mimeType == t {
			allowed = true
			break
		}
	}
	if !allowed {
		return nil, errors.New("不支持的图片类型")
	}

	if _, err := src.Seek(0, 0); err != nil {
		return nil, errors.New("文件处理失败")
	}

	if ext == "" {
		ext = extensionFromImageMimeType(mimeType)
	}

	// 存储 key：[uploadPath/]YYYY/MM/DD/uuid.ext
	datePath := time.Now().Format("2006/01/02")
	var storageKey string
	if s.config.UploadPath != "" {
		uploadPath := strings.Trim(s.config.UploadPath, "/")
		storageKey = fmt.Sprintf("%s/%s/%s%s", uploadPath, datePath, uuid.New().String(), ext)
	} else {
		storageKey = fmt.Sprintf("%s/%s%s", datePath, uuid.New().String(), ext)
	}

	input := &s3.PutObjectInput{
		Bucket:      aws.String(s.config.Bucket),
		Key:         aws.String(storageKey),
		Body:        src,
		ContentType: aws.String(mimeType),
	}
	if s.config.PublicRead {
		input.ACL = types.ObjectCannedACLPublicRead
	}

	if _, err := s.s3Client.PutObject(ctx, input); err != nil {
		return nil, fmt.Errorf("上传文件失败: %v", err)
	}

	return &StorageUploadResult{
		StorageKey: storageKey,
		FileSize:   file.Size,
		MimeType:   mimeType,
	}, nil
}

// ResolveURL 解析存储 key 的真实访问地址：CDN 优先，否则生成预签名 URL（兼容私有桶）。
func (s *StorageService) ResolveURL(ctx context.Context, storageKey string) (string, error) {
	if s.config.CDN != "" {
		cdn := strings.TrimSuffix(s.config.CDN, "/")
		return fmt.Sprintf("%s/%s", cdn, storageKey), nil
	}

	presignClient := s3.NewPresignClient(s.s3Client)
	req, err := presignClient.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.config.Bucket),
		Key:    aws.String(storageKey),
	}, s3.WithPresignExpires(presignedURLExpiry))
	if err != nil {
		return "", fmt.Errorf("生成访问地址失败: %v", err)
	}

	return req.URL, nil
}

// extensionFromImageMimeType 从 MIME 类型推导图片扩展名。
func extensionFromImageMimeType(mimeType string) string {
	switch mimeType {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	case "image/svg+xml":
		return ".svg"
	default:
		return ""
	}
}
