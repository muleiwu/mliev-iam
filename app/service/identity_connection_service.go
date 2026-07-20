package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/iam/mliev-iam/app/service/protocol"
	directoryprotocol "cnb.cool/mliev/iam/mliev-iam/app/service/protocol/directory"
	eventprotocol "cnb.cool/mliev/iam/mliev-iam/app/service/protocol/event"
	"cnb.cool/mliev/iam/mliev-iam/internal/pkg/credentialcrypto"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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
		if err := s.saveCredentialInputs(tx, connection.ID, req.Credentials); err != nil {
			return err
		}
		if connection.Status == model.ConnectionStatusEnabled {
			_, err := NewIdentityConnectionService(tx).testCapabilities(context.Background(), connection, connection.Capabilities, true)
			return err
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("创建身份连接失败: %w", err)
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
	oldStatus := connection.Status
	oldCapabilities := append(model.StringList(nil), connection.Capabilities...)
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
		merged := make(model.JSONObject, len(connection.Config)+len(req.Config))
		for key, value := range connection.Config {
			merged[key] = value
		}
		for key, value := range req.Config {
			if incoming, ok := value.(map[string]any); ok {
				if current, ok := merged[key].(map[string]any); ok {
					nested := make(map[string]any, len(current)+len(incoming))
					for k, v := range current {
						nested[k] = v
					}
					for k, v := range incoming {
						nested[k] = v
					}
					merged[key] = nested
					continue
				}
			}
			merged[key] = value
		}
		connection.Config = merged
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
		if err := s.saveCredentialInputs(tx, connection.ID, req.Credentials); err != nil {
			return err
		}
		if connection.Status != model.ConnectionStatusEnabled || !connection.HasCapability(model.ConnectionCapabilityDirectorySync) {
			if err := NewSyncJobService(tx).CancelConnectionPending(connection.ID, "连接已禁用或 directory_sync 能力已移除"); err != nil {
				return err
			}
		}
		if connection.Status != model.ConnectionStatusEnabled {
			return nil
		}
		capabilitiesToTest := model.StringList{}
		if oldStatus != model.ConnectionStatusEnabled || req.Config != nil || len(req.Credentials) > 0 {
			capabilitiesToTest = connection.Capabilities
		} else if req.Capabilities != nil {
			for _, capability := range connection.Capabilities {
				if !oldCapabilities.Contains(capability) {
					capabilitiesToTest = append(capabilitiesToTest, capability)
				}
			}
		}
		if len(capabilitiesToTest) > 0 {
			_, err := NewIdentityConnectionService(tx).testCapabilities(context.Background(), connection, capabilitiesToTest, true)
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
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
				status := model.ConnectionStatusDisabled
				if _, err := service.Update(id, &dto.UpdateIdentityConnectionRequest{Status: &status}); err != nil {
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
	_, err = s.testCapabilities(context.Background(), connection, connection.Capabilities, false)
	return err
}

func (s *IdentityConnectionService) TestCapabilities(ctx context.Context, id uint) ([]model.IdentityConnectionHealthCheck, error) {
	connection, err := s.dao.GetByID(id)
	if err != nil {
		return nil, err
	}
	return s.testCapabilities(ctx, connection, connection.Capabilities, false)
}

func (s *IdentityConnectionService) testCapabilities(ctx context.Context, connection *model.IdentityConnection, capabilities model.StringList, failFast bool) ([]model.IdentityConnectionHealthCheck, error) {
	results := make([]model.IdentityConnectionHealthCheck, 0, len(capabilities))
	var firstErr error
	for _, raw := range capabilities {
		capability := model.ConnectionCapability(raw)
		now := time.Now()
		check := model.IdentityConnectionHealthCheck{ConnectionID: connection.ID, Capability: capability, Status: model.CapabilityHealthHealthy, CheckedAt: now, Details: model.JSONObject{}}
		var checkErr error
		switch capability {
		case model.ConnectionCapabilityAuthentication:
			var adapter protocol.AuthenticationAdapter
			adapter, checkErr = BuildAuthenticationAdapter(s.db, connection, nil)
			if checkErr == nil {
				checkErr = adapter.ValidateConfig()
			}
		case model.ConnectionCapabilityDirectorySync:
			var credentials map[string]any
			credentials, checkErr = s.CredentialValues(connection.ID, model.CredentialPurposeDirectory)
			if checkErr == nil {
				var adapter directoryprotocol.Adapter
				adapter, checkErr = directoryprotocol.NewAdapter(connection, credentials, nil)
				if checkErr == nil {
					var diagnostic *directoryprotocol.Diagnostic
					diagnostic, checkErr = adapter.ValidateDirectoryConfig(ctx)
					if diagnostic != nil {
						raw, _ := json.Marshal(diagnostic)
						_ = json.Unmarshal(raw, &check.Details)
					}
				}
			}
		case model.ConnectionCapabilityEvents:
			var credentials map[string]any
			credentials, checkErr = s.CredentialValues(connection.ID, model.CredentialPurposeEvents)
			if checkErr == nil {
				_, checkErr = eventprotocol.NewAdapter(connection, credentials)
			}
		default:
			checkErr = fmt.Errorf("未知连接能力 %q", capability)
		}
		if checkErr != nil {
			check.Status = model.CapabilityHealthFailed
			check.ErrorMessage = checkErr.Error()
			if firstErr == nil {
				firstErr = checkErr
			}
		}
		if err := s.db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "connection_id"}, {Name: "capability"}}, DoUpdates: clause.AssignmentColumns([]string{"status", "error_message", "details_json", "checked_at", "updated_at"})}).Create(&check).Error; err != nil {
			return results, err
		}
		results = append(results, check)
		if checkErr != nil && failFast {
			return results, fmt.Errorf("%s capability 校验失败: %w", capability, checkErr)
		}
	}
	now := time.Now()
	overall := "healthy"
	errorText := ""
	if firstErr != nil {
		overall = "failed"
		errorText = firstErr.Error()
	}
	_ = s.db.Model(&model.IdentityConnection{}).Where("id = ?", connection.ID).Updates(map[string]any{"last_checked_at": now, "last_check_status": overall, "last_check_error": errorText}).Error
	return results, firstErr
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
		} else if err != gorm.ErrRecordNotFound {
			return fmt.Errorf("读取已有 %s 凭据失败，拒绝覆盖: %w", purpose, err)
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
		keyVersion := cipher.ActiveKeyVersion()
		envelope, err := cipher.Encrypt(plaintext, credentialcrypto.AAD(connectionID, purposeString, keyVersion))
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
	var health []model.IdentityConnectionHealthCheck
	if s.db.Migrator().HasTable(&model.IdentityConnectionHealthCheck{}) {
		if err := s.db.Where("connection_id = ?", connection.ID).Order("capability ASC").Find(&health).Error; err != nil {
			return nil, err
		}
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
		Health: health,
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
	allowed := map[model.ConnectionCategory]map[string]bool{
		model.ConnectionCategorySocial: {
			"generic": true, "wechat": true, "wechat_miniprogram": true, "qq": true,
			"alipay": true, "github": true, "google": true, "oidc": true,
		},
		model.ConnectionCategoryEnterprise: {"feishu": true, "wecom": true, "dingtalk": true},
	}
	if !allowed[category][providerType] {
		return fmt.Errorf("连接类别 %s 不支持 providerType %q", category, providerType)
	}
	for _, capability := range capabilities {
		switch model.ConnectionCapability(capability) {
		case model.ConnectionCapabilityAuthentication, model.ConnectionCapabilityDirectorySync, model.ConnectionCapabilityEvents:
		default:
			return fmt.Errorf("非法连接能力 %q", capability)
		}
		if category == model.ConnectionCategorySocial && capability != string(model.ConnectionCapabilityAuthentication) {
			return fmt.Errorf("社交连接只能启用 authentication 能力")
		}
	}
	if status != "" && status != model.ConnectionStatusDisabled && status != model.ConnectionStatusEnabled {
		return fmt.Errorf("非法连接状态 %q", status)
	}
	return nil
}
