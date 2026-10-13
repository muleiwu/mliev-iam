package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/open/go-web/pkg/helper"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

var ErrInvalidSessionQuery = errors.New("无效的会话状态")

type LoginSessionService struct{ db *gorm.DB }

func NewLoginSessionService(db *gorm.DB) *LoginSessionService { return &LoginSessionService{db: db} }

func (s *LoginSessionService) Create(userID uint, method, ip, userAgent string, until time.Time) (*model.LoginSession, error) {
	return s.CreateWithProof(userID, method, ip, userAgent, until, nil)
}

func (s *LoginSessionService) CreateWithProof(userID uint, method, ip, userAgent string, until time.Time, proof *TwoFactorProof) (*model.LoginSession, error) {
	device, browser, os := loginDevice(userAgent)
	now := time.Now()
	session := &model.LoginSession{ID: uuid.NewString(), UserID: userID, LoginMethod: method, LoginIP: ip, LastIP: ip, Device: device, Browser: browser, OS: os, CreatedAt: now, LastActiveAt: now, ExpiresAt: until}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := dao.LockLoginUser(tx, userID); err != nil {
			return err
		}
		var user model.User
		if err := tx.First(&user, userID).Error; err != nil {
			return err
		}
		if !user.CanLogin() {
			return dao.ErrLoginSessionInvalid
		}
		if user.TwoFactorEnabled && !proof.validFor(&user) {
			return ErrTwoFactorRequired
		}
		if proof != nil && !proof.validFor(&user) {
			return ErrTwoFactorExpired
		}
		session.TwoFactorVersion = user.TwoFactorVersion
		if proof != nil {
			session.TwoFactorMethod = proof.method
			session.TwoFactorAt = &proof.at
		}
		session.CreatedAt = time.Now()
		session.LastActiveAt = session.CreatedAt
		return tx.Create(session).Error
	})
	return session, err
}

// Revoke uses the caller's transaction so password/account changes and
// revocation commit together. Revoking access never changes account status.
func RevokeLoginSessions(db *gorm.DB, userID uint, onlyID, exceptID, reason string, actor *uint) error {
	if err := dao.LockLoginUser(db, userID); err != nil {
		return err
	}
	query := db.Model(&model.LoginSession{}).Where("user_id = ?", userID)
	if onlyID != "" {
		query = query.Where("id = ?", onlyID)
	}
	if exceptID != "" {
		query = query.Where("id <> ?", exceptID)
	}
	var ids []string
	if err := query.Pluck("id", &ids).Error; err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	now := time.Now()
	if err := db.Model(&model.LoginSession{}).Where("id IN ? AND ended_at IS NULL AND expires_at > ?", ids, now).Updates(map[string]any{"ended_at": now, "end_reason": reason, "ended_by": actor}).Error; err != nil {
		return err
	}
	if err := db.Model(&model.OIDCToken{}).Where("login_session_id IN ?", ids).Update("is_revoked", true).Error; err != nil {
		return err
	}
	return db.Model(&model.AuthorizationCode{}).Where("login_session_id IN ?", ids).Update("expires_at", now).Error
}

func (s *LoginSessionService) Revoke(userID uint, onlyID, exceptID, reason string, actor *uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error { return RevokeLoginSessions(tx, userID, onlyID, exceptID, reason, actor) })
}

type LoginSessionQuery struct {
	Page     int    `form:"page"`
	PageSize int    `form:"pageSize"`
	UserID   uint   `form:"userId"`
	Status   string `form:"status"`
	IP       string `form:"ip"`
	ClientID string `form:"clientId"`
}
type SessionApplication struct {
	ClientID string `json:"clientId"`
	Name     string `json:"name"`
}
type LoginSessionItem struct {
	model.LoginSession
	Username     string               `json:"username"`
	Status       string               `json:"status"`
	IsCurrent    bool                 `json:"isCurrent"`
	Applications []SessionApplication `json:"applications"`
}
type LoginSessionList struct {
	Items []LoginSessionItem `json:"items"`
	Total int64              `json:"total"`
	Page  int                `json:"page"`
	Size  int                `json:"size"`
}

func (s *LoginSessionService) List(q LoginSessionQuery, currentID string) (*LoginSessionList, error) {
	if q.Page < 1 {
		q.Page = 1
	}
	if q.PageSize < 1 {
		q.PageSize = 20
	}
	if q.PageSize > 100 {
		q.PageSize = 100
	}
	if q.Status == "" {
		q.Status = "active"
	}
	query := s.db.Model(&model.LoginSession{})
	if q.UserID != 0 {
		query = query.Where("user_id = ?", q.UserID)
	}
	if q.IP != "" {
		query = query.Where("login_ip = ? OR last_ip = ?", q.IP, q.IP)
	}
	if q.ClientID != "" {
		query = query.Where("id IN (?)", s.db.Model(&model.OIDCToken{}).Select("login_session_id").Where("client_id = ?", q.ClientID))
	}
	now := time.Now()
	switch q.Status {
	case "active":
		query = query.Where("ended_at IS NULL AND expires_at > ?", now)
	case "history":
		query = query.Where("ended_at IS NOT NULL OR expires_at <= ?", now)
	case "logged_out":
		query = query.Where("ended_at IS NOT NULL AND end_reason = ?", "logout")
	case "revoked":
		query = query.Where("ended_at IS NOT NULL AND end_reason <> ?", "logout")
	case "expired":
		query = query.Where("ended_at IS NULL AND expires_at <= ?", now)
	case "all":
	default:
		return nil, ErrInvalidSessionQuery
	}
	result := &LoginSessionList{Items: []LoginSessionItem{}, Page: q.Page, Size: q.PageSize}
	if err := query.Count(&result.Total).Error; err != nil {
		return nil, err
	}
	var sessions []model.LoginSession
	if err := query.Order("last_active_at DESC, id DESC").Offset((q.Page - 1) * q.PageSize).Limit(q.PageSize).Find(&sessions).Error; err != nil {
		return nil, err
	}
	ids, userIDs := []string{}, []uint{}
	for _, session := range sessions {
		ids = append(ids, session.ID)
		userIDs = append(userIDs, session.UserID)
	}
	if len(ids) == 0 {
		return result, nil
	}
	var users []struct {
		ID       uint
		Username string
	}
	if err := s.db.Table("users").Select("id, username").Where("id IN ?", userIDs).Find(&users).Error; err != nil {
		return nil, err
	}
	names := map[uint]string{}
	for _, user := range users {
		names[user.ID] = user.Username
	}
	var apps []struct {
		LoginSessionID string
		ClientID       string
		Name           string
	}
	if err := s.db.Table("oidc_tokens AS t").Select("DISTINCT t.login_session_id, t.client_id, a.name").Joins("LEFT JOIN applications a ON a.uuid = t.client_id").Where("t.login_session_id IN ?", ids).Scan(&apps).Error; err != nil {
		return nil, err
	}
	appMap := map[string][]SessionApplication{}
	for _, app := range apps {
		name := app.Name
		if name == "" {
			name = app.ClientID
		}
		appMap[app.LoginSessionID] = append(appMap[app.LoginSessionID], SessionApplication{ClientID: app.ClientID, Name: name})
	}
	for _, session := range sessions {
		applications := appMap[session.ID]
		if applications == nil {
			applications = []SessionApplication{}
		}
		result.Items = append(result.Items, LoginSessionItem{LoginSession: session, Username: names[session.UserID], Status: session.Status(now), IsCurrent: session.ID == currentID, Applications: applications})
	}
	return result, nil
}

func (s *LoginSessionService) Cleanup(ctx context.Context, now time.Time) error {
	cutoff := now.Add(-90 * 24 * time.Hour)
	for {
		var ids []string
		if err := s.db.WithContext(ctx).Model(&model.LoginSession{}).Where("ended_at < ? OR (ended_at IS NULL AND expires_at < ?)", cutoff, cutoff).Limit(500).Pluck("id", &ids).Error; err != nil {
			return err
		}
		if len(ids) == 0 {
			return nil
		}
		if err := s.db.WithContext(ctx).Where("id IN ?", ids).Delete(&model.LoginSession{}).Error; err != nil {
			return err
		}
	}
}

type LoginSessionCleanupServer struct {
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func (s *LoginSessionCleanupServer) Run() error {
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			if err := NewLoginSessionService(helper.GetDatabase()).Cleanup(ctx, time.Now()); err != nil && ctx.Err() == nil {
				helper.GetLogger().Error("清理登录会话失败: " + err.Error())
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return nil
}
func (s *LoginSessionCleanupServer) Stop() error {
	if s.cancel != nil {
		s.cancel()
		s.wg.Wait()
	}
	return nil
}

func loginDevice(ua string) (device, browser, os string) {
	device, browser, os = "未知设备", "未知浏览器", "未知系统"
	switch {
	case strings.Contains(ua, "iPad"):
		device, os = "平板", "iPadOS"
	case strings.Contains(ua, "iPhone"):
		device, os = "手机", "iOS"
	case strings.Contains(ua, "Android"):
		device, os = "手机", "Android"
	case strings.Contains(ua, "Windows"):
		device, os = "电脑", "Windows"
	case strings.Contains(ua, "Macintosh"):
		device, os = "电脑", "macOS"
	case strings.Contains(ua, "Linux"):
		device, os = "电脑", "Linux"
	}
	switch {
	case strings.Contains(ua, "MicroMessenger"):
		browser = "微信"
	case strings.Contains(ua, "Edg/"):
		browser = "Edge"
	case strings.Contains(ua, "OPR/"):
		browser = "Opera"
	case strings.Contains(ua, "Firefox/") || strings.Contains(ua, "FxiOS/"):
		browser = "Firefox"
	case strings.Contains(ua, "Chrome/") || strings.Contains(ua, "CriOS/"):
		browser = "Chrome"
	case strings.Contains(ua, "Safari/"):
		browser = "Safari"
	}
	return
}
