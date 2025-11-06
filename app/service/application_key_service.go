package service

import (
	"errors"
	"fmt"

	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/iam/mliev-iam/internal/pkg/jwt"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ApplicationKeyService 应用密钥服务
type ApplicationKeyService struct {
	keyDAO     *dao.ApplicationOIDCKeyDAO
	keyFactory *jwt.KeyManagerFactory
	db         *gorm.DB
}

// NewApplicationKeyService 创建应用密钥服务
func NewApplicationKeyService(db *gorm.DB) *ApplicationKeyService {
	return &ApplicationKeyService{
		keyDAO:     dao.NewApplicationOIDCKeyDAO(db),
		keyFactory: jwt.NewKeyManagerFactory(),
		db:         db,
	}
}

// GenerateKeyForApplication 为应用生成密钥
func (s *ApplicationKeyService) GenerateKeyForApplication(appID uint, algorithm string, keySize int) (*model.ApplicationOIDCKey, error) {
	// 1. 验证算法是否支持
	if !s.keyFactory.IsAlgorithmSupported(algorithm) {
		return nil, fmt.Errorf("不支持的算法: %s", algorithm)
	}

	// 2. 生成密钥ID
	keyID, err := uuid.NewV7()
	if err != nil {
		keyID = uuid.New() // 回退到v4
	}
	keyIDStr := keyID.String()

	// 3. 创建密钥管理器
	keyManager, err := s.keyFactory.CreateKeyManager(algorithm, keyIDStr, keySize)
	if err != nil {
		return nil, fmt.Errorf("创建密钥管理器失败: %w", err)
	}

	// 4. 生成密钥
	if err := keyManager.GenerateKey(); err != nil {
		return nil, fmt.Errorf("生成密钥失败: %w", err)
	}

	// 5. 导出密钥
	privateKey, err := keyManager.ExportPrivateKey()
	if err != nil {
		return nil, fmt.Errorf("导出私钥失败: %w", err)
	}

	publicKey, _ := keyManager.ExportPublicKey() // 对称加密可能返回空

	// 6. 创建密钥记录
	key := &model.ApplicationOIDCKey{
		ApplicationID: appID,
		KeyID:         keyIDStr,
		Algorithm:     keyManager.GetAlgorithm(),
		KeyType:       string(keyManager.GetKeyType()),
		PrivateKey:    privateKey, // TODO: 加密存储
		PublicKey:     publicKey,
		KeySize:       keyManager.GetKeySize(),
		IsPrimary:     true, // 默认设为主密钥
	}

	// 7. 保存到数据库（事务：先取消其他主密钥，再保存新密钥）
	err = s.db.Transaction(func(tx *gorm.DB) error {
		// 取消该应用下所有密钥的主密钥状态
		if err := tx.Model(&model.ApplicationOIDCKey{}).
			Where("application_id = ?", appID).
			Update("is_primary", false).Error; err != nil {
			return err
		}

		// 保存新密钥
		if err := tx.Create(key).Error; err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("保存密钥失败: %w", err)
	}

	return key, nil
}

// GetPrimaryKeyForApplication 获取应用的主密钥
func (s *ApplicationKeyService) GetPrimaryKeyForApplication(appID uint) (*model.ApplicationOIDCKey, error) {
	key, err := s.keyDAO.GetPrimaryByApplicationID(appID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("应用没有主密钥")
		}
		return nil, fmt.Errorf("获取主密钥失败: %w", err)
	}

	return key, nil
}

// GetPrimaryKeyManagerForApplication 获取应用的主密钥管理器
func (s *ApplicationKeyService) GetPrimaryKeyManagerForApplication(appID uint) (jwt.KeyManager, error) {
	// 1. 获取主密钥记录
	key, err := s.GetPrimaryKeyForApplication(appID)
	if err != nil {
		return nil, err
	}

	// 2. 加载密钥管理器
	return s.LoadKeyManager(key)
}

// GetKeyByKeyID 根据KeyID获取密钥
func (s *ApplicationKeyService) GetKeyByKeyID(keyID string) (*model.ApplicationOIDCKey, error) {
	key, err := s.keyDAO.GetByKeyID(keyID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("密钥不存在")
		}
		return nil, fmt.Errorf("获取密钥失败: %w", err)
	}

	return key, nil
}

// GetKeyManagerByKeyID 根据KeyID获取密钥管理器
func (s *ApplicationKeyService) GetKeyManagerByKeyID(keyID string) (jwt.KeyManager, error) {
	// 1. 获取密钥记录
	key, err := s.GetKeyByKeyID(keyID)
	if err != nil {
		return nil, err
	}

	// 2. 加载密钥管理器
	return s.LoadKeyManager(key)
}

// LoadKeyManager 从数据库记录加载密钥管理器
func (s *ApplicationKeyService) LoadKeyManager(key *model.ApplicationOIDCKey) (jwt.KeyManager, error) {
	// 构建元数据
	metadata := &jwt.KeyManagerMetadata{
		KeyID:      key.KeyID,
		Algorithm:  key.Algorithm,
		KeyType:    jwt.KeyType(key.KeyType),
		KeySize:    key.KeySize,
		PrivateKey: key.PrivateKey, // TODO: 解密
		PublicKey:  key.PublicKey,
	}

	// 加载密钥管理器
	keyManager, err := s.keyFactory.LoadKeyManager(metadata)
	if err != nil {
		return nil, fmt.Errorf("加载密钥管理器失败: %w", err)
	}

	return keyManager, nil
}

// ListKeysByApplication 获取应用的所有密钥
func (s *ApplicationKeyService) ListKeysByApplication(appID uint) ([]*model.ApplicationOIDCKey, error) {
	keys, err := s.keyDAO.ListByApplicationID(appID)
	if err != nil {
		return nil, fmt.Errorf("获取密钥列表失败: %w", err)
	}

	return keys, nil
}

// SetPrimaryKey 设置主密钥
func (s *ApplicationKeyService) SetPrimaryKey(appID, keyID uint) error {
	return s.keyDAO.SetPrimary(appID, keyID)
}

// DeleteKey 删除密钥
func (s *ApplicationKeyService) DeleteKey(keyID uint) error {
	// TODO: 检查是否为主密钥，如果是，可能需要先设置其他密钥为主密钥
	return s.keyDAO.Delete(keyID)
}

// DeleteKeysByApplication 删除应用的所有密钥
func (s *ApplicationKeyService) DeleteKeysByApplication(appID uint) error {
	return s.keyDAO.DeleteByApplicationID(appID)
}

// GetDefaultAlgorithm 获取默认算法
func (s *ApplicationKeyService) GetDefaultAlgorithm() string {
	return "RS256"
}

// GetDefaultKeySize 获取默认密钥大小
func (s *ApplicationKeyService) GetDefaultKeySize(algorithm string) int {
	switch algorithm {
	case "RS256", "RS384":
		return 2048
	case "RS512":
		return 4096
	case "HS256":
		return 256
	case "HS384":
		return 384
	case "HS512":
		return 512
	default:
		return 2048
	}
}
