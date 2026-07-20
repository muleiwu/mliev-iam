package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

var agreementCodePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{1,63}$`)

var (
	ErrAgreementNotFound = errors.New("协议不存在")
	ErrAgreementDisabled = errors.New("协议未启用")
)

// AgreementService 协议服务。
type AgreementService struct {
	db           *gorm.DB
	agreementDAO *dao.AgreementDAO
	appDAO       *dao.ApplicationDAO
	acceptDAO    *dao.AgreementAcceptanceDAO
}

// NewAgreementService 创建协议服务。
func NewAgreementService(db *gorm.DB) *AgreementService {
	return &AgreementService{
		db:           db,
		agreementDAO: dao.NewAgreementDAO(db),
		appDAO:       dao.NewApplicationDAO(db),
		acceptDAO:    dao.NewAgreementAcceptanceDAO(db),
	}
}

func defaultGlobalAgreements() []model.Agreement {
	return []model.Agreement{
		{
			Scope:         string(model.AgreementScopeGlobal),
			ApplicationID: 0,
			Code:          model.AgreementCodeTerms,
			Title:         "服务协议",
			ContentMD:     "",
			Enabled:       false,
			Sort:          10,
		},
		{
			Scope:         string(model.AgreementScopeGlobal),
			ApplicationID: 0,
			Code:          model.AgreementCodePrivacy,
			Title:         "隐私协议",
			ContentMD:     "",
			Enabled:       false,
			Sort:          20,
		},
	}
}

func isGlobalAgreementCode(code string) bool {
	return code == model.AgreementCodeTerms || code == model.AgreementCodePrivacy
}

func normalizeAgreementCode(code string) string {
	return strings.ToLower(strings.TrimSpace(code))
}

func validateApplicationAgreementCode(code string) error {
	if !agreementCodePattern.MatchString(code) {
		return fmt.Errorf("协议代码只能包含小写字母、数字、下划线和中划线，且长度为2-64个字符")
	}
	return nil
}

func agreementKey(agreement *model.Agreement) string {
	if agreement.Scope == string(model.AgreementScopeApplication) {
		return string(model.AgreementScopeApplication) + ":" + agreement.Code
	}
	return string(model.AgreementScopeGlobal) + ":" + agreement.Code
}

// AgreementKeys 返回协议 key 列表。
func AgreementKeys(agreements []*model.Agreement) []string {
	keys := make([]string, 0, len(agreements))
	for _, agreement := range agreements {
		keys = append(keys, agreementKey(agreement))
	}
	return keys
}

func (s *AgreementService) ensureGlobalAgreements() error {
	for _, item := range defaultGlobalAgreements() {
		if _, err := s.agreementDAO.GetByScopeCode(item.Scope, item.ApplicationID, item.Code); err == nil {
			continue
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		agreement := item
		if err := s.agreementDAO.Create(&agreement); err != nil {
			return err
		}
	}
	return nil
}

// ListGlobalAgreements 获取全局协议。
func (s *AgreementService) ListGlobalAgreements(includeDisabled bool) ([]dto.AgreementResponse, error) {
	if err := s.ensureGlobalAgreements(); err != nil {
		return nil, fmt.Errorf("初始化全局协议失败: %w", err)
	}

	agreements, err := s.agreementDAO.ListByScope(string(model.AgreementScopeGlobal), 0, includeDisabled)
	if err != nil {
		return nil, fmt.Errorf("获取全局协议失败: %w", err)
	}
	return s.toAgreementResponses(agreements), nil
}

// GetGlobalAgreement 获取单个全局协议。
func (s *AgreementService) GetGlobalAgreement(code string) (*dto.AgreementResponse, error) {
	code = normalizeAgreementCode(code)
	if !isGlobalAgreementCode(code) {
		return nil, fmt.Errorf("不支持的全局协议代码: %s", code)
	}
	if err := s.ensureGlobalAgreements(); err != nil {
		return nil, fmt.Errorf("初始化全局协议失败: %w", err)
	}
	agreement, err := s.agreementDAO.GetByScopeCode(string(model.AgreementScopeGlobal), 0, code)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAgreementNotFound
		}
		return nil, err
	}
	resp := s.toAgreementResponse(agreement)
	return &resp, nil
}

// UpsertGlobalAgreement 更新全局协议。
func (s *AgreementService) UpsertGlobalAgreement(code string, req *dto.AgreementUpsertRequest) (*dto.AgreementResponse, error) {
	code = normalizeAgreementCode(code)
	if !isGlobalAgreementCode(code) {
		return nil, fmt.Errorf("不支持的全局协议代码: %s", code)
	}
	if err := s.ensureGlobalAgreements(); err != nil {
		return nil, fmt.Errorf("初始化全局协议失败: %w", err)
	}

	agreement, err := s.agreementDAO.GetByScopeCode(string(model.AgreementScopeGlobal), 0, code)
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		agreement = &model.Agreement{
			Scope:         string(model.AgreementScopeGlobal),
			ApplicationID: 0,
			Code:          code,
		}
	}

	agreement.Title = strings.TrimSpace(req.Title)
	if agreement.Title == "" {
		return nil, fmt.Errorf("协议标题不能为空")
	}
	agreement.ContentMD = req.ContentMD
	agreement.Enabled = req.Enabled
	agreement.Sort = req.Sort

	if agreement.ID == 0 {
		if err := s.agreementDAO.Create(agreement); err != nil {
			return nil, err
		}
	} else if err := s.agreementDAO.Update(agreement); err != nil {
		return nil, err
	}

	resp := s.toAgreementResponse(agreement)
	return &resp, nil
}

// ListApplicationAgreements 获取应用协议列表。
func (s *AgreementService) ListApplicationAgreements(appID uint, includeDisabled bool) ([]dto.AgreementResponse, error) {
	if _, err := s.appDAO.GetByID(appID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrApplicationNotFound
		}
		return nil, err
	}
	agreements, err := s.agreementDAO.ListByScope(string(model.AgreementScopeApplication), appID, includeDisabled)
	if err != nil {
		return nil, fmt.Errorf("获取应用协议失败: %w", err)
	}
	return s.toAgreementResponses(agreements), nil
}

// CreateApplicationAgreement 创建应用协议。
func (s *AgreementService) CreateApplicationAgreement(appID uint, req *dto.AgreementUpsertRequest) (*dto.AgreementResponse, error) {
	if _, err := s.appDAO.GetByID(appID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrApplicationNotFound
		}
		return nil, err
	}

	code := normalizeAgreementCode(req.Code)
	if err := validateApplicationAgreementCode(code); err != nil {
		return nil, err
	}
	if exists, err := s.agreementDAO.ExistsByScopeCodeExceptID(string(model.AgreementScopeApplication), appID, code, 0); err != nil {
		return nil, err
	} else if exists {
		return nil, fmt.Errorf("协议代码已存在: %s", code)
	}

	title := strings.TrimSpace(req.Title)
	if title == "" {
		return nil, fmt.Errorf("协议标题不能为空")
	}

	agreement := &model.Agreement{
		Scope:         string(model.AgreementScopeApplication),
		ApplicationID: appID,
		Code:          code,
		Title:         title,
		ContentMD:     req.ContentMD,
		Enabled:       req.Enabled,
		Sort:          req.Sort,
	}
	if err := s.agreementDAO.Create(agreement); err != nil {
		return nil, err
	}
	resp := s.toAgreementResponse(agreement)
	return &resp, nil
}

// UpdateApplicationAgreement 更新应用协议。
func (s *AgreementService) UpdateApplicationAgreement(appID, agreementID uint, req *dto.AgreementUpsertRequest) (*dto.AgreementResponse, error) {
	agreement, err := s.agreementDAO.GetByID(agreementID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAgreementNotFound
		}
		return nil, err
	}
	if agreement.Scope != string(model.AgreementScopeApplication) || agreement.ApplicationID != appID {
		return nil, ErrAgreementNotFound
	}

	code := normalizeAgreementCode(req.Code)
	if code == "" {
		code = agreement.Code
	}
	if err := validateApplicationAgreementCode(code); err != nil {
		return nil, err
	}
	if exists, err := s.agreementDAO.ExistsByScopeCodeExceptID(string(model.AgreementScopeApplication), appID, code, agreement.ID); err != nil {
		return nil, err
	} else if exists {
		return nil, fmt.Errorf("协议代码已存在: %s", code)
	}

	title := strings.TrimSpace(req.Title)
	if title == "" {
		return nil, fmt.Errorf("协议标题不能为空")
	}

	agreement.Code = code
	agreement.Title = title
	agreement.ContentMD = req.ContentMD
	agreement.Enabled = req.Enabled
	agreement.Sort = req.Sort
	if err := s.agreementDAO.Update(agreement); err != nil {
		return nil, err
	}
	resp := s.toAgreementResponse(agreement)
	return &resp, nil
}

// DeleteApplicationAgreement 删除应用协议。
func (s *AgreementService) DeleteApplicationAgreement(appID, agreementID uint) error {
	agreement, err := s.agreementDAO.GetByID(agreementID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrAgreementNotFound
		}
		return err
	}
	if agreement.Scope != string(model.AgreementScopeApplication) || agreement.ApplicationID != appID {
		return ErrAgreementNotFound
	}
	return s.agreementDAO.Delete(agreement)
}

// GetPublicGlobalAgreement 获取公开全局协议。
func (s *AgreementService) GetPublicGlobalAgreement(code string) (*dto.AgreementPublicResponse, error) {
	code = normalizeAgreementCode(code)
	if !isGlobalAgreementCode(code) {
		return nil, ErrAgreementNotFound
	}
	if err := s.ensureGlobalAgreements(); err != nil {
		return nil, err
	}
	agreement, err := s.agreementDAO.GetByScopeCode(string(model.AgreementScopeGlobal), 0, code)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAgreementNotFound
		}
		return nil, err
	}
	if !agreement.Enabled {
		return nil, ErrAgreementDisabled
	}
	return s.toPublicResponse(agreement, "", ""), nil
}

// GetPublicApplicationAgreement 获取公开应用协议。
func (s *AgreementService) GetPublicApplicationAgreement(clientID, code string) (*dto.AgreementPublicResponse, error) {
	clientID = strings.TrimSpace(clientID)
	code = normalizeAgreementCode(code)
	if clientID == "" || code == "" {
		return nil, fmt.Errorf("缺少 client_id 或 code 参数")
	}

	app, err := s.appDAO.GetByClientID(clientID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrApplicationNotFound
		}
		return nil, err
	}
	if !app.IsActive {
		return nil, ErrApplicationDisabled
	}

	agreement, err := s.agreementDAO.GetByScopeCode(string(model.AgreementScopeApplication), app.ID, code)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAgreementNotFound
		}
		return nil, err
	}
	if !agreement.Enabled {
		return nil, ErrAgreementDisabled
	}
	return s.toPublicResponse(agreement, app.Name, app.Icon), nil
}

// BuildClientAgreementLinks 构造 OIDC 前端协议链接。
func (s *AgreementService) BuildClientAgreementLinks(app *model.Application) ([]dto.ClientAgreementLink, error) {
	required, err := s.ListRequiredRegistrationAgreements(applicationIDPtr(app))
	if err != nil {
		return nil, err
	}
	links := make([]dto.ClientAgreementLink, 0, len(required))
	for _, agreement := range required {
		links = append(links, dto.ClientAgreementLink{
			Key:   agreementKey(agreement),
			Scope: agreement.Scope,
			Code:  agreement.Code,
			Title: agreement.Title,
			URL:   s.publicURLForAgreement(agreement, app),
		})
	}
	return links, nil
}

func applicationIDPtr(app *model.Application) *uint {
	if app == nil {
		return nil
	}
	return &app.ID
}

func (s *AgreementService) publicURLForAgreement(agreement *model.Agreement, app *model.Application) string {
	if agreement.Scope == string(model.AgreementScopeApplication) {
		clientID := ""
		if app != nil {
			clientID = app.UUID
		}
		return "/auth/#/agreements/application?client_id=" + clientID + "&code=" + agreement.Code
	}
	if agreement.Code == model.AgreementCodePrivacy {
		return "/auth/#/agreements/privacy"
	}
	return "/auth/#/agreements/terms"
}

// ResolveApplicationIDFromSession 根据 OIDC session 解析应用 ID。
func (s *AgreementService) ResolveApplicationIDFromSession(ctx context.Context, sessionID string, redisClient *redis.Client) (*uint, error) {
	if strings.TrimSpace(sessionID) == "" {
		return nil, nil
	}
	sessionService := NewOIDCSessionService(redisClient)
	session, err := sessionService.GetSession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("无效的会话ID或会话已过期")
	}
	app, err := s.appDAO.GetByClientID(session.ClientID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrApplicationNotFound
		}
		return nil, err
	}
	return &app.ID, nil
}

// ListRequiredRegistrationAgreements 获取注册必选协议。
func (s *AgreementService) ListRequiredRegistrationAgreements(appID *uint) ([]*model.Agreement, error) {
	if err := s.ensureGlobalAgreements(); err != nil {
		return nil, err
	}
	global, err := s.agreementDAO.ListByScope(string(model.AgreementScopeGlobal), 0, false)
	if err != nil {
		return nil, err
	}

	required := make([]*model.Agreement, 0, len(global)+4)
	required = append(required, global...)
	if appID != nil {
		appAgreements, err := s.agreementDAO.ListByScope(string(model.AgreementScopeApplication), *appID, false)
		if err != nil {
			return nil, err
		}
		required = append(required, appAgreements...)
	}

	sort.SliceStable(required, func(i, j int) bool {
		if required[i].Scope != required[j].Scope {
			return required[i].Scope == string(model.AgreementScopeGlobal)
		}
		if required[i].Sort != required[j].Sort {
			return required[i].Sort < required[j].Sort
		}
		return required[i].ID < required[j].ID
	})
	return required, nil
}

// EnsureAgreementsAccepted 校验协议 key 是否完整。
func (s *AgreementService) EnsureAgreementsAccepted(required []*model.Agreement, acceptedKeys []string) error {
	if len(required) == 0 {
		return nil
	}
	accepted := map[string]struct{}{}
	for _, key := range acceptedKeys {
		accepted[strings.TrimSpace(key)] = struct{}{}
	}
	missing := make([]string, 0)
	for _, agreement := range required {
		key := agreementKey(agreement)
		if _, ok := accepted[key]; !ok {
			missing = append(missing, agreement.Title)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("请阅读并同意%s", strings.Join(missing, "、"))
	}
	return nil
}

// ValidateRegistrationAcceptance 获取并校验注册协议。
func (s *AgreementService) ValidateRegistrationAcceptance(appID *uint, acceptedKeys []string) ([]*model.Agreement, error) {
	required, err := s.ListRequiredRegistrationAgreements(appID)
	if err != nil {
		return nil, err
	}
	if err := s.EnsureAgreementsAccepted(required, acceptedKeys); err != nil {
		return nil, err
	}
	return required, nil
}

// RecordAcceptances 记录用户协议同意。
func (s *AgreementService) RecordAcceptances(userID uint, appID *uint, agreements []*model.Agreement) error {
	if len(agreements) == 0 {
		return nil
	}
	applicationID := uint(0)
	if appID != nil {
		applicationID = *appID
	}
	acceptedAt := time.Now()
	records := make([]model.AgreementAcceptance, 0, len(agreements))
	for _, agreement := range agreements {
		hash := sha256.Sum256([]byte(agreement.ContentMD))
		records = append(records, model.AgreementAcceptance{
			UserID:        userID,
			ApplicationID: applicationID,
			AgreementID:   agreement.ID,
			Scope:         agreement.Scope,
			Code:          agreement.Code,
			Title:         agreement.Title,
			ContentSHA256: hex.EncodeToString(hash[:]),
			AcceptedAt:    acceptedAt,
		})
	}
	return s.acceptDAO.CreateBatch(records)
}

func (s *AgreementService) toAgreementResponse(agreement *model.Agreement) dto.AgreementResponse {
	return dto.AgreementResponse{
		ID:            agreement.ID,
		Scope:         agreement.Scope,
		ApplicationID: agreement.ApplicationID,
		Code:          agreement.Code,
		Title:         agreement.Title,
		ContentMD:     agreement.ContentMD,
		Enabled:       agreement.Enabled,
		Sort:          agreement.Sort,
		CreatedAt:     agreement.CreatedAt.Format(time.RFC3339),
		UpdatedAt:     agreement.UpdatedAt.Format(time.RFC3339),
	}
}

func (s *AgreementService) toAgreementResponses(agreements []*model.Agreement) []dto.AgreementResponse {
	responses := make([]dto.AgreementResponse, 0, len(agreements))
	for _, agreement := range agreements {
		responses = append(responses, s.toAgreementResponse(agreement))
	}
	return responses
}

func (s *AgreementService) toPublicResponse(agreement *model.Agreement, appName, appIcon string) *dto.AgreementPublicResponse {
	return &dto.AgreementPublicResponse{
		Scope:     agreement.Scope,
		Code:      agreement.Code,
		Title:     agreement.Title,
		ContentMD: agreement.ContentMD,
		UpdatedAt: agreement.UpdatedAt.Format(time.RFC3339),
		AppName:   appName,
		AppIcon:   appIcon,
	}
}
