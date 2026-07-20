package service

import (
	"context"
	"errors"
	"fmt"
	"mime/multipart"

	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ErrAttachmentNotFound 附件不存在。
var ErrAttachmentNotFound = errors.New("附件不存在")

// AttachmentService 附件服务：上传到对象存储并登记附件表。
type AttachmentService struct {
	attachmentDao *dao.AttachmentDao
}

// NewAttachmentService 创建附件服务实例
func NewAttachmentService(db *gorm.DB) *AttachmentService {
	return &AttachmentService{
		attachmentDao: dao.NewAttachmentDao(db),
	}
}

// UploadAvatar 上传头像：写入对象存储 + 创建附件记录，返回 /attachments/{uuid} 相对地址。
func (s *AttachmentService) UploadAvatar(ctx context.Context, file *multipart.FileHeader, uploaderUserID uint) (*dto.UploadAvatarResponse, error) {
	storageService, err := NewStorageService(ctx)
	if err != nil {
		return nil, err
	}

	result, err := storageService.UploadAvatarFile(ctx, file)
	if err != nil {
		return nil, err
	}

	return s.saveUploadedImage(file, result, uploaderUserID, model.AttachmentBizTypeAvatar)
}

// UploadApplicationIcon 上传应用图标并登记独立的附件业务类型。
func (s *AttachmentService) UploadApplicationIcon(ctx context.Context, file *multipart.FileHeader, uploaderUserID uint) (*dto.UploadAvatarResponse, error) {
	storageService, err := NewStorageService(ctx)
	if err != nil {
		return nil, err
	}

	result, err := storageService.UploadApplicationIconFile(ctx, file)
	if err != nil {
		return nil, err
	}

	return s.saveUploadedImage(file, result, uploaderUserID, model.AttachmentBizTypeApplicationIcon)
}

func (s *AttachmentService) saveUploadedImage(file *multipart.FileHeader, result *StorageUploadResult, uploaderUserID uint, bizType string) (*dto.UploadAvatarResponse, error) {

	attachment := &model.Attachment{
		UUID:           uuid.NewString(),
		FileName:       file.Filename,
		FileSize:       result.FileSize,
		MimeType:       result.MimeType,
		StorageType:    model.AttachmentStorageTypeS3,
		StorageKey:     result.StorageKey,
		BizType:        bizType,
		UploaderUserID: uploaderUserID,
	}
	if err := s.attachmentDao.Create(attachment); err != nil {
		return nil, fmt.Errorf("保存附件记录失败: %w", err)
	}

	return &dto.UploadAvatarResponse{
		UUID:     attachment.UUID,
		URL:      "/attachments/" + attachment.UUID,
		FileName: attachment.FileName,
		FileSize: attachment.FileSize,
		MimeType: attachment.MimeType,
	}, nil
}

// ResolveRedirectURL 根据附件 UUID 解析真实存储地址（302 跳转目标）。
func (s *AttachmentService) ResolveRedirectURL(ctx context.Context, attachmentUUID string) (string, error) {
	attachment, err := s.attachmentDao.GetByUUID(attachmentUUID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", ErrAttachmentNotFound
		}
		return "", fmt.Errorf("查询附件失败: %w", err)
	}

	storageService, err := NewStorageService(ctx)
	if err != nil {
		return "", err
	}

	return storageService.ResolveURL(ctx, attachment.StorageKey)
}
