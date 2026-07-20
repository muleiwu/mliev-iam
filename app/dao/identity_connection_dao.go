package dao

import (
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"gorm.io/gorm"
)

type IdentityConnectionDAO struct{ db *gorm.DB }

func NewIdentityConnectionDAO(db *gorm.DB) *IdentityConnectionDAO {
	return &IdentityConnectionDAO{db: db}
}

func (d *IdentityConnectionDAO) Create(connection *model.IdentityConnection) error {
	return d.db.Create(connection).Error
}

func (d *IdentityConnectionDAO) GetByID(id uint) (*model.IdentityConnection, error) {
	var connection model.IdentityConnection
	if err := d.db.First(&connection, id).Error; err != nil {
		return nil, err
	}
	return &connection, nil
}

func (d *IdentityConnectionDAO) GetByIdentifier(identifier string) (*model.IdentityConnection, error) {
	var connection model.IdentityConnection
	if err := d.db.Where("identifier = ?", identifier).First(&connection).Error; err != nil {
		return nil, err
	}
	return &connection, nil
}

func (d *IdentityConnectionDAO) ExistsIdentifier(identifier string, excludeID uint) (bool, error) {
	query := d.db.Model(&model.IdentityConnection{}).Where("identifier = ?", identifier)
	if excludeID != 0 {
		query = query.Where("id <> ?", excludeID)
	}
	var count int64
	return count > 0, query.Count(&count).Error
}

func (d *IdentityConnectionDAO) List(page, pageSize int, category, providerType, capability, status string) ([]model.IdentityConnection, int64, error) {
	query := d.db.Model(&model.IdentityConnection{})
	if category != "" {
		query = query.Where("category = ?", category)
	}
	if providerType != "" {
		query = query.Where("provider_type = ?", providerType)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if capability != "" {
		// capabilities is JSON/JSONB in server databases and JSON text in SQLite.
		if d.db.Dialector.Name() == "postgres" {
			query = query.Where("CAST(capabilities AS TEXT) LIKE ?", "%\""+capability+"\"%")
		} else {
			query = query.Where("capabilities LIKE ?", "%\""+capability+"\"%")
		}
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 20
	}
	var connections []model.IdentityConnection
	err := query.Order("sort_order ASC, id ASC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&connections).Error
	return connections, total, err
}

func (d *IdentityConnectionDAO) GetEnabledAuthenticationConnections() ([]model.IdentityConnection, error) {
	var connections []model.IdentityConnection
	err := d.db.Where("status = ?", model.ConnectionStatusEnabled).
		Order("category ASC, sort_order ASC, id ASC").Find(&connections).Error
	if err != nil {
		return nil, err
	}
	result := make([]model.IdentityConnection, 0, len(connections))
	for _, connection := range connections {
		if connection.HasCapability(model.ConnectionCapabilityAuthentication) {
			result = append(result, connection)
		}
	}
	return result, nil
}

func (d *IdentityConnectionDAO) Save(connection *model.IdentityConnection) error {
	return d.db.Save(connection).Error
}

func (d *IdentityConnectionDAO) Delete(id uint) error {
	return d.db.Delete(&model.IdentityConnection{}, id).Error
}

func (d *IdentityConnectionDAO) Credentials(connectionID uint) ([]model.IdentityConnectionCredential, error) {
	var credentials []model.IdentityConnectionCredential
	err := d.db.Where("connection_id = ?", connectionID).Find(&credentials).Error
	return credentials, err
}

func (d *IdentityConnectionDAO) Credential(connectionID uint, purpose model.CredentialPurpose) (*model.IdentityConnectionCredential, error) {
	var credential model.IdentityConnectionCredential
	if err := d.db.Where("connection_id = ? AND purpose = ?", connectionID, purpose).First(&credential).Error; err != nil {
		return nil, err
	}
	return &credential, nil
}

func (d *IdentityConnectionDAO) UpsertCredential(credential *model.IdentityConnectionCredential) error {
	var existing model.IdentityConnectionCredential
	err := d.db.Where("connection_id = ? AND purpose = ?", credential.ConnectionID, credential.Purpose).First(&existing).Error
	if err == nil {
		credential.ID = existing.ID
		credential.CreatedAt = existing.CreatedAt
		return d.db.Save(credential).Error
	}
	if err != gorm.ErrRecordNotFound {
		return err
	}
	return d.db.Create(credential).Error
}

func (d *IdentityConnectionDAO) CountWhere(field string, value any) (int64, error) {
	var count int64
	query := d.db.Model(&model.IdentityConnection{})
	if field != "" {
		query = query.Where(field+" = ?", value)
	}
	err := query.Count(&count).Error
	return count, err
}

func (d *IdentityConnectionDAO) CountPrincipals(connectionID uint) (int64, error) {
	var count int64
	query := d.db.Model(&model.ExternalPrincipal{})
	if connectionID != 0 {
		query = query.Where("connection_id = ?", connectionID)
	}
	err := query.Count(&count).Error
	return count, err
}
