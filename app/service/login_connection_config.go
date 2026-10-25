package service

import (
	"fmt"

	"cnb.cool/mliev/iam/mliev-iam/app/model"
)

func (s *ApplicationService) validateLoginConnections(input, previous *model.LoginConnectionConfig) (*model.LoginConnectionConfig, error) {
	if input == nil {
		return nil, nil
	}
	result := &model.LoginConnectionConfig{Mode: input.Mode, ConnectionIDs: []uint{}}
	switch input.Mode {
	case model.LoginConnectionsInherit:
		return result, nil
	case model.LoginConnectionsCustom:
	default:
		return nil, fmt.Errorf("登录方式展示模式无效")
	}
	retained := map[uint]bool{}
	if previous != nil && previous.Mode == model.LoginConnectionsCustom {
		for _, id := range previous.ConnectionIDs {
			retained[id] = true
		}
	}
	seen := map[uint]bool{}
	newIDs := []uint{}
	for _, id := range input.ConnectionIDs {
		if id == 0 {
			return nil, fmt.Errorf("登录方式连接 ID 无效")
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		result.ConnectionIDs = append(result.ConnectionIDs, id)
		if !retained[id] {
			newIDs = append(newIDs, id)
		}
	}
	if len(newIDs) == 0 {
		return result, nil
	}
	var connections []model.IdentityConnection
	if err := s.db.Where("id IN ?", newIDs).Find(&connections).Error; err != nil {
		return nil, err
	}
	valid := map[uint]bool{}
	for _, connection := range connections {
		valid[connection.ID] = connection.IsEnabled() && connection.HasCapability(model.ConnectionCapabilityAuthentication)
	}
	for _, id := range newIDs {
		if !valid[id] {
			return nil, fmt.Errorf("身份连接 %d 不可用于登录展示，请选择已启用且具备认证能力的连接", id)
		}
	}
	return result, nil
}
