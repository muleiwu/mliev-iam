package service

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/iam/mliev-iam/app/service/protocol"
	"cnb.cool/mliev/iam/mliev-iam/internal/pkg/credentialcrypto"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type IdentityConnectionService struct {
	db  *gorm.DB
	dao *dao.IdentityConnectionDAO
}

func NewIdentityConnectionService(db *gorm.DB) *IdentityConnectionService {
	return &IdentityConnectionService{db: db, dao: dao.NewIdentityConnectionDAO(db)}
}

func (s *IdentityConnectionService) Create(req *dto.CreateIdentityConnectionRequest) (*dto.IdentityConnectionResponse, error) {
	if err := validateIdentityConnectionFields(req.Identifier, req.Category, req.ProviderType, req.Capabilities, req.Status); err != nil {
		return nil, err
	}
	exists, err := s.dao.ExistsIdentifier(req.Identifier, 0)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, fmt.Errorf("连接标识 %q 已存在", req.Identifier)
	}
	capabilities := req.Capabilities
	if len(capabilities) == 0 {
		capabilities = model.StringList{string(model.ConnectionCapabilityAuthentication)}
	}
	status := req.Status
	if status == "" {
		status = model.ConnectionStatusDisabled
	}
	connection := &model.IdentityConnection{
		Name: req.Name, Identifier: req.Identifier, Category: req.Category,
		ProviderType: req.ProviderType, Description: req.Description, Capabilities: capabilities,
		Config: model.JSONObject(req.Config), Icon: req.Icon, SortOrder: req.SortOrder,
		Status: status, IsDefault: req.IsDefault, Remark: req.Remark,
	}
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := dao.NewIdentityConnectionDAO(tx).Create(connection); err != nil {
			return err
		}
		return s.saveCredentialInputs(tx, connection.ID, req.Credentials)
	})
	if err != nil {
		return nil, fmt.Errorf("创建身份连接失败: %w", err)
	}
	if connection.Status == model.ConnectionStatusEnabled {
		if err := s.Test(connection.ID); err != nil {
			_ = s.db.Model(connection).Update("status", model.ConnectionStatusDisabled).Error
			return nil, fmt.Errorf("连接配置校验失败，已保持禁用: %w", err)
		}
	}
	return s.Get(connection.ID)
}

func (s *IdentityConnectionService) Get(id uint) (*dto.IdentityConnectionResponse, error) {
	connection, err := s.dao.GetByID(id)
	if err != nil {
		return nil, err
	}
	return s.toResponse(connection)
}

func (s *IdentityConnectionService) GetByIdentifier(identifier string) (*model.IdentityConnection, error) {
	return s.dao.GetByIdentifier(identifier)
}

func (s *IdentityConnectionService) List(page, pageSize int, category, providerType, capability, status string) ([]*dto.IdentityConnectionResponse, int64, error) {
	connections, total, err := s.dao.List(page, pageSize, category, providerType, capability, status)
	if err != nil {
		return nil, 0, err
	}
	responses := make([]*dto.IdentityConnectionResponse, 0, len(connections))
	for i := range connections {
		response, err := s.toResponse(&connections[i])
		if err != nil {
			return nil, 0, err
		}
		responses = append(responses, response)
	}
	return responses, total, nil
}

func (s *IdentityConnectionService) PublicList() ([]dto.PublicIdentityConnectionInfo, error) {
	connections, err := s.dao.GetEnabledAuthenticationConnections()
	if err != nil {
		return nil, err
	}
	result := make([]dto.PublicIdentityConnectionInfo, 0, len(connections))
	for _, connection := range connections {
		result = append(result, dto.PublicIdentityConnectionInfo{
			Identifier: connection.Identifier, Name: connection.Name, Category: connection.Category,
			ProviderType: connection.ProviderType, Capabilities: connection.Capabilities,
			Icon: connection.Icon, SortOrder: connection.SortOrder, IsDefault: connection.IsDefault,
		})
	}
	return result, nil
}

func (s *IdentityConnectionService) Update(id uint, req *dto.UpdateIdentityConnectionRequest) (*dto.IdentityConnectionResponse, error) {
	connection, err := s.dao.GetByID(id)
	if err != nil {
		return nil, err
	}
	if req.Name != nil {
		connection.Name = *req.Name
	}
	if req.Identifier != nil {
		exists, err := s.dao.ExistsIdentifier(*req.Identifier, id)
		if err != nil {
			return nil, err
		}
		if exists {
			return nil, fmt.Errorf("连接标识 %q 已存在", *req.Identifier)
		}
		connection.Identifier = *req.Identifier
	}
	if req.Category != nil {
		connection.Category = *req.Category
	}
	if req.ProviderType != nil {
		connection.ProviderType = *req.ProviderType
	}
	if req.Description != nil {
		connection.Description = *req.Description
	}
	if req.Capabilities != nil {
		connection.Capabilities = *req.Capabilities
	}
	if req.Config != nil {
		connection.Config = model.JSONObject(req.Config)
	}
	if req.Icon != nil {
		connection.Icon = *req.Icon
	}
	if req.SortOrder != nil {
		connection.SortOrder = *req.SortOrder
	}
	if req.Status != nil {
		connection.Status = *req.Status
	}
	if req.IsDefault != nil {
		connection.IsDefault = *req.IsDefault
	}
	if req.Remark != nil {
		connection.Remark = *req.Remark
	}
	if err := validateIdentityConnectionFields(connection.Identifier, connection.Category, connection.ProviderType, connection.Capabilities, connection.Status); err != nil {
		return nil, err
	}
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := dao.NewIdentityConnectionDAO(tx).Save(connection); err != nil {
			return err
		}
		return s.saveCredentialInputs(tx, connection.ID, req.Credentials)
	})
	if err != nil {
		return nil, err
	}
	if connection.Status == model.ConnectionStatusEnabled {
		if err := s.Test(connection.ID); err != nil {
			_ = s.db.Model(connection).Update("status", model.ConnectionStatusDisabled).Error
			return nil, fmt.Errorf("连接配置校验失败: %w", err)
		}
	}
	return s.Get(id)
}

func (s *IdentityConnectionService) Delete(id uint) error {
	count, err := s.dao.CountPrincipals(id)
	if err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("连接仍有 %d 个外部身份，不能删除；请先禁用并解除绑定", count)
	}
	return s.dao.Delete(id)
}

func (s *IdentityConnectionService) Batch(req *dto.IdentityConnectionBatchRequest) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		service := NewIdentityConnectionService(tx)
		for _, id := range req.IDs {
			switch req.Operation {
			case "enable":
				status := model.ConnectionStatusEnabled
				if _, err := service.Update(id, &dto.UpdateIdentityConnectionRequest{Status: &status}); err != nil {
					return err
				}
			case "disable":
				if err := tx.Model(&model.IdentityConnection{}).Where("id = ?", id).Update("status", model.ConnectionStatusDisabled).Error; err != nil {
					return err
				}
			case "delete":
				if err := service.Delete(id); err != nil {
					return err
				}
			default:
				return fmt.Errorf("不支持的批量操作 %q", req.Operation)
			}
		}
		return nil
	})
}

func (s *IdentityConnectionService) Stats() (*dto.IdentityConnectionStatsResponse, error) {
	total, err := s.dao.CountWhere("", nil)
	if err != nil {
		return nil, err
	}
	enabled, err := s.dao.CountWhere("status", model.ConnectionStatusEnabled)
	if err != nil {
		return nil, err
	}
	social, err := s.dao.CountWhere("category", model.ConnectionCategorySocial)
	if err != nil {
		return nil, err
	}
	enterprise, err := s.dao.CountWhere("category", model.ConnectionCategoryEnterprise)
	if err != nil {
		return nil, err
	}
	bindings, err := s.dao.CountPrincipals(0)
	if err != nil {
		return nil, err
	}
	return &dto.IdentityConnectionStatsResponse{
		TotalConnections: total, EnabledConnections: enabled, SocialConnections: social,
		EnterpriseConnections: enterprise, TotalBindings: bindings,
	}, nil
}

func (s *IdentityConnectionService) Test(id uint) error {
	connection, err := s.dao.GetByID(id)
	if err != nil {
		return err
	}
	adapter, err := BuildAuthenticationAdapter(s.db, connection, nil)
	if err == nil {
		err = adapter.ValidateConfig()
	}
	now := time.Now()
	updates := map[string]any{"last_checked_at": now}
	if err != nil {
		updates["last_check_status"] = "failed"
		updates["last_check_error"] = err.Error()
	} else {
		updates["last_check_status"] = "healthy"
		updates["last_check_error"] = ""
	}
	_ = s.db.Model(&model.IdentityConnection{}).Where("id = ?", id).Updates(updates).Error
	return err
}

func BuildAuthenticationAdapter(db *gorm.DB, connection *model.IdentityConnection, redisClient *redis.Client) (protocol.AuthenticationAdapter, error) {
	if connection == nil {
		return nil, fmt.Errorf("身份连接不能为空")
	}
	credentials, err := NewIdentityConnectionService(db).CredentialValues(connection.ID, model.CredentialPurposeAuthentication)
	if err != nil {
		return nil, fmt.Errorf("读取认证凭据失败: %w", err)
	}
	return protocol.NewAuthenticationAdapter(connection, credentials, redisClient)
}

func (s *IdentityConnectionService) CredentialValues(connectionID uint, purpose model.CredentialPurpose) (map[string]any, error) {
	credential, err := s.dao.Credential(connectionID, purpose)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return map[string]any{}, nil
		}
		return nil, err
	}
	cipher, err := credentialcrypto.FromEnvironment()
	if err != nil {
		return nil, err
	}
	plaintext, err := cipher.Decrypt(credentialcrypto.Envelope{
		Ciphertext: credential.Ciphertext, Nonce: credential.Nonce,
		Algorithm: credential.Algorithm, KeyVersion: credential.KeyVersion,
	}, credentialcrypto.AAD(connectionID, string(purpose), credential.KeyVersion))
	if err != nil {
		return nil, err
	}
	var values map[string]any
	if err := json.Unmarshal(plaintext, &values); err != nil {
		return nil, fmt.Errorf("解析连接凭据失败: %w", err)
	}
	return values, nil
}

// SaveCredentials is shared by capability-specific configuration pages. It
// preserves existing secret fields when the UI sends an empty placeholder.
func (s *IdentityConnectionService) SaveCredentials(connectionID uint, purpose model.CredentialPurpose, values map[string]any) error {
	if _, err := s.dao.GetByID(connectionID); err != nil {
		return err
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		return s.saveCredentialInputs(tx, connectionID, dto.IdentityConnectionCredentialInput{
			string(purpose): values,
		})
	})
}

func (s *IdentityConnectionService) saveCredentialInputs(tx *gorm.DB, connectionID uint, inputs dto.IdentityConnectionCredentialInput) error {
	if len(inputs) == 0 {
		return nil
	}
	cipher, err := credentialcrypto.FromEnvironment()
	if err != nil {
		return err
	}
	connectionDAO := dao.NewIdentityConnectionDAO(tx)
	for purposeString, input := range inputs {
		purpose := model.CredentialPurpose(purposeString)
		if purpose != model.CredentialPurposeAuthentication && purpose != model.CredentialPurposeDirectory && purpose != model.CredentialPurposeEvents {
			return fmt.Errorf("非法凭据用途 %q", purposeString)
		}
		merged := make(map[string]any)
		if existing, err := NewIdentityConnectionService(tx).CredentialValues(connectionID, purpose); err == nil {
			for key, value := range existing {
				merged[key] = value
			}
		}
		for key, value := range input {
			if text, ok := value.(string); ok && text == "" {
				continue
			}
			merged[key] = value
		}
		plaintext, err := json.Marshal(merged)
		if err != nil {
			return err
		}
		envelope, err := cipher.Encrypt(plaintext, credentialcrypto.AAD(connectionID, purposeString, credentialcrypto.KeyVersion))
		if err != nil {
			return err
		}
		credential := &model.IdentityConnectionCredential{
			ConnectionID: connectionID, Purpose: purpose, Ciphertext: envelope.Ciphertext,
			Nonce: envelope.Nonce, Algorithm: envelope.Algorithm, KeyVersion: envelope.KeyVersion,
			RotatedAt: time.Now(),
		}
		if err := connectionDAO.UpsertCredential(credential); err != nil {
			return err
		}
	}
	return nil
}

func (s *IdentityConnectionService) toResponse(connection *model.IdentityConnection) (*dto.IdentityConnectionResponse, error) {
	credentials, err := s.dao.Credentials(connection.ID)
	if err != nil {
		return nil, err
	}
	summaries := make([]dto.IdentityConnectionCredentialSummary, 0, len(credentials))
	for _, credential := range credentials {
		rotatedAt := credential.RotatedAt
		summaries = append(summaries, dto.IdentityConnectionCredentialSummary{
			Purpose: credential.Purpose, Configured: true, KeyVersion: credential.KeyVersion, RotatedAt: &rotatedAt,
		})
	}
	return &dto.IdentityConnectionResponse{
		ID: connection.ID, Name: connection.Name, Identifier: connection.Identifier,
		Category: connection.Category, ProviderType: connection.ProviderType,
		Description: connection.Description, Capabilities: connection.Capabilities,
		Config: map[string]any(connection.Config), CredentialState: summaries,
		Icon: connection.Icon, SortOrder: connection.SortOrder, Status: connection.Status,
		IsDefault: connection.IsDefault, LastCheckedAt: connection.LastCheckedAt,
		LastCheckStatus: connection.LastCheckStatus, LastCheckError: connection.LastCheckError,
		Remark: connection.Remark, CreatedAt: connection.CreatedAt, UpdatedAt: connection.UpdatedAt,
	}, nil
}

func validateIdentityConnectionFields(identifier string, category model.ConnectionCategory, providerType string, capabilities model.StringList, status model.ConnectionStatus) error {
	identifier = strings.TrimSpace(identifier)
	if identifier == "" || strings.ContainsAny(identifier, "/?# ") {
		return fmt.Errorf("连接标识不能为空且不能包含空格或 URL 保留字符")
	}
	if category != model.ConnectionCategorySocial && category != model.ConnectionCategoryEnterprise {
		return fmt.Errorf("非法连接类别 %q", category)
	}
	if strings.TrimSpace(providerType) == "" {
		return fmt.Errorf("providerType 不能为空")
	}
	for _, capability := range capabilities {
		switch model.ConnectionCapability(capability) {
		case model.ConnectionCapabilityAuthentication, model.ConnectionCapabilityDirectorySync, model.ConnectionCapabilityEvents:
		default:
			return fmt.Errorf("非法连接能力 %q", capability)
		}
	}
	if status != "" && status != model.ConnectionStatusDisabled && status != model.ConnectionStatusEnabled {
		return fmt.Errorf("非法连接状态 %q", status)
	}
	return nil
}
