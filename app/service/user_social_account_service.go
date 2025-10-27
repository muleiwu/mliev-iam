package service

import (
	"fmt"

	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"gorm.io/gorm"
)

// UserSocialAccountService 用户社交账号管理服务
type UserSocialAccountService struct {
	db         *gorm.DB
	accountDAO *dao.UserSocialAccountDAO
}

// NewUserSocialAccountService 创建服务实例
func NewUserSocialAccountService(db *gorm.DB) *UserSocialAccountService {
	return &UserSocialAccountService{
		db:         db,
		accountDAO: dao.NewUserSocialAccountDAO(db),
	}
}

// GetUserSocialAccounts 获取用户的所有社交账号
func (s *UserSocialAccountService) GetUserSocialAccounts(userID uint) ([]*dto.UserSocialAccountResponse, error) {
	accounts, err := s.accountDAO.GetByUserID(userID)
	if err != nil {
		return nil, fmt.Errorf("获取用户社交账号失败: %w", err)
	}

	responses := make([]*dto.UserSocialAccountResponse, 0, len(accounts))
	for _, account := range accounts {
		var userInfo map[string]any
		if account.UserInfo != nil {
			userInfo = account.UserInfo
		}

		var providerName, providerIcon string
		if account.Provider != nil {
			providerName = account.Provider.Name
			providerIcon = account.Provider.Icon
		}

		responses = append(responses, &dto.UserSocialAccountResponse{
			ID:           account.ID,
			UserID:       account.UserID,
			ProviderID:   account.ProviderID,
			ProviderName: providerName,
			ProviderIcon: providerIcon,
			OpenID:       account.OpenID,
			UnionID:      account.UnionID,
			UserInfo:     userInfo,
			LastUsedAt:   account.LastUsedAt,
			LoginCount:   account.LoginCount,
			CreatedAt:    account.CreatedAt,
		})
	}

	return responses, nil
}

// GetAccountByID 根据ID获取社交账号详情
func (s *UserSocialAccountService) GetAccountByID(accountID uint) (*dto.UserSocialAccountResponse, error) {
	account, err := s.accountDAO.GetByID(accountID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("社交账号不存在")
		}
		return nil, fmt.Errorf("获取社交账号失败: %w", err)
	}

	var userInfo map[string]any
	if account.UserInfo != nil {
		userInfo = account.UserInfo
	}

	return &dto.UserSocialAccountResponse{
		ID:         account.ID,
		UserID:     account.UserID,
		ProviderID: account.ProviderID,
		OpenID:     account.OpenID,
		UnionID:    account.UnionID,
		UserInfo:   userInfo,
		LastUsedAt: account.LastUsedAt,
		LoginCount: account.LoginCount,
		CreatedAt:  account.CreatedAt,
	}, nil
}
