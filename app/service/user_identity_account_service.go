package service

import (
	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"gorm.io/gorm"
)

type UserIdentityAccountService struct {
	accountDAO *dao.UserIdentityAccountDAO
}

func NewUserIdentityAccountService(db *gorm.DB) *UserIdentityAccountService {
	return &UserIdentityAccountService{accountDAO: dao.NewUserIdentityAccountDAO(db)}
}

func (s *UserIdentityAccountService) GetUserIdentityAccounts(userID uint) ([]*dto.UserIdentityAccountResponse, error) {
	accounts, err := s.accountDAO.GetByUserID(userID)
	if err != nil {
		return nil, err
	}
	responses := make([]*dto.UserIdentityAccountResponse, 0, len(accounts))
	for _, account := range accounts {
		response := &dto.UserIdentityAccountResponse{
			ID:             account.ID,
			UserID:         account.UserID,
			ProviderID:     account.ProviderID,
			ExternalUserID: account.ExternalUserID,
			UnionID:        account.UnionID,
			TenantID:       account.TenantID,
			UserInfo:       account.UserInfo,
			LastUsedAt:     account.LastUsedAt,
			LoginCount:     account.LoginCount,
			CreatedAt:      account.CreatedAt,
		}
		if account.Provider != nil {
			response.ProviderName = account.Provider.Name
			response.ProviderIcon = account.Provider.Icon
		}
		responses = append(responses, response)
	}
	return responses, nil
}
