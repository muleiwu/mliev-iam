package service

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/dao"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type PrincipalIdentifierInput struct {
	Namespace model.IdentifierNamespace
	Value     string
	Primary   bool
}

type PrincipalConflictError struct{ ConflictID uint }

func (e *PrincipalConflictError) Error() string {
	return fmt.Sprintf("外部标识命中多个 principal，已创建冲突 #%d，禁止自动合并", e.ConflictID)
}

type ExternalPrincipalResolver struct{ db *gorm.DB }

func NewExternalPrincipalResolver(db *gorm.DB) *ExternalPrincipalResolver {
	return &ExternalPrincipalResolver{db: db}
}

func normalizePrincipalIdentifiers(inputs []PrincipalIdentifierInput) []PrincipalIdentifierInput {
	seen := map[string]bool{}
	result := make([]PrincipalIdentifierInput, 0, len(inputs))
	for _, input := range inputs {
		if input.Value == "" {
			continue
		}
		key := string(input.Namespace) + "\x00" + input.Value
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, input)
	}
	priority := func(namespace model.IdentifierNamespace) int {
		switch namespace {
		case model.IdentifierNamespaceDirectoryUserID:
			return 0
		case model.IdentifierNamespaceLoginSubject:
			return 1
		case model.IdentifierNamespaceOpenID, model.IdentifierNamespaceOpenUserID:
			return 2
		case model.IdentifierNamespaceUnionID:
			return 3
		default:
			return 4
		}
	}
	sort.SliceStable(result, func(i, j int) bool { return priority(result[i].Namespace) < priority(result[j].Namespace) })
	return result
}

// Resolve evaluates the complete identifier set in one connection. It never
// selects the first match when multiple principals are found.
func (r *ExternalPrincipalResolver) Resolve(connectionID uint, inputs []PrincipalIdentifierInput, createIfMissing bool) (*model.ExternalPrincipal, error) {
	return r.resolve(connectionID, inputs, createIfMissing, true)
}

// ResolveForDirectory uses the same identifier/conflict semantics as login,
// while permitting an authoritative directory refresh of a blocked principal.
func (r *ExternalPrincipalResolver) ResolveForDirectory(connectionID uint, inputs []PrincipalIdentifierInput, createIfMissing bool) (*model.ExternalPrincipal, error) {
	return r.resolve(connectionID, inputs, createIfMissing, false)
}

func (r *ExternalPrincipalResolver) resolve(connectionID uint, inputs []PrincipalIdentifierInput, createIfMissing, rejectBlocked bool) (*model.ExternalPrincipal, error) {
	inputs = normalizePrincipalIdentifiers(inputs)
	if len(inputs) == 0 {
		return nil, errors.New("外部身份没有可解析标识")
	}
	query := r.db.Where("connection_id = ?", connectionID)
	condition := r.db.Where("1 = 0")
	for _, input := range inputs {
		condition = condition.Or("namespace = ? AND value = ?", input.Namespace, input.Value)
	}
	var matches []model.ExternalIdentifier
	if err := query.Where(condition).Find(&matches).Error; err != nil {
		return nil, err
	}
	principalSet := map[uint]bool{}
	for _, match := range matches {
		principalSet[match.PrincipalID] = true
	}
	if len(principalSet) > 1 {
		conflict, err := r.createConflict(connectionID, inputs, principalSet)
		if err != nil {
			return nil, err
		}
		return nil, &PrincipalConflictError{ConflictID: conflict.ID}
	}
	var principal model.ExternalPrincipal
	for principalID := range principalSet {
		if err := r.db.First(&principal, principalID).Error; err != nil {
			return nil, err
		}
		if rejectBlocked && principal.Status == model.PrincipalStatusBlocked {
			return nil, errors.New("该企业身份已被停用")
		}
		if err := r.EnsureIdentifiers(r.db, &principal, inputs); err != nil {
			return nil, err
		}
		return &principal, nil
	}
	if !createIfMissing {
		return nil, gorm.ErrRecordNotFound
	}
	principal = model.ExternalPrincipal{ConnectionID: connectionID, Status: model.PrincipalStatusUnmatched}
	if err := r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&principal).Error; err != nil {
			return err
		}
		return NewExternalPrincipalResolver(tx).EnsureIdentifiers(tx, &principal, inputs)
	}); err != nil {
		return nil, err
	}
	return &principal, nil
}

func (r *ExternalPrincipalResolver) EnsureIdentifiers(db *gorm.DB, principal *model.ExternalPrincipal, inputs []PrincipalIdentifierInput) error {
	for _, input := range normalizePrincipalIdentifiers(inputs) {
		var existing model.ExternalIdentifier
		err := db.Where("connection_id = ? AND namespace = ? AND value = ?", principal.ConnectionID, input.Namespace, input.Value).First(&existing).Error
		if err == nil {
			if existing.PrincipalID != principal.ID {
				conflict, createErr := NewExternalPrincipalResolver(db).createConflict(principal.ConnectionID, inputs, map[uint]bool{principal.ID: true, existing.PrincipalID: true})
				if createErr != nil {
					return createErr
				}
				return &PrincipalConflictError{ConflictID: conflict.ID}
			}
			if existing.IsPrimary != input.Primary {
				if err := db.Model(&existing).Update("is_primary", input.Primary).Error; err != nil {
					return err
				}
			}
			continue
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		record := model.ExternalIdentifier{ConnectionID: principal.ConnectionID, PrincipalID: principal.ID, Namespace: input.Namespace, Value: input.Value, IsPrimary: input.Primary}
		if err := db.Create(&record).Error; err != nil {
			return fmt.Errorf("保存外部标识 %s 失败: %w", input.Namespace, err)
		}
	}
	return nil
}

func (r *ExternalPrincipalResolver) createConflict(connectionID uint, inputs []PrincipalIdentifierInput, principalSet map[uint]bool) (*model.ExternalPrincipalConflict, error) {
	identifiers := model.JSONObject{}
	for _, input := range normalizePrincipalIdentifiers(inputs) {
		identifiers[string(input.Namespace)] = appendStringValue(identifiers[string(input.Namespace)], input.Value)
	}
	ids := make(model.StringList, 0, len(principalSet))
	for id := range principalSet {
		ids = append(ids, strconv.FormatUint(uint64(id), 10))
	}
	sort.Strings(ids)
	var existing model.ExternalPrincipalConflict
	err := r.db.Where("connection_id = ? AND status = ? AND candidate_principal_ids_json = ?", connectionID, model.PrincipalConflictPending, ids).First(&existing).Error
	if err == nil {
		return &existing, nil
	}
	conflict := model.ExternalPrincipalConflict{ConnectionID: connectionID, IdentifierSet: identifiers, CandidatePrincipalIDs: ids, Status: model.PrincipalConflictPending}
	if err := r.db.Create(&conflict).Error; err != nil {
		return nil, err
	}
	return &conflict, nil
}

func appendStringValue(existing any, value string) []string {
	values, _ := existing.([]string)
	return append(values, value)
}

func (r *ExternalPrincipalResolver) ListConflicts(connectionID uint, status string, page, pageSize int) ([]model.ExternalPrincipalConflict, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 20
	}
	query := r.db.Model(&model.ExternalPrincipalConflict{})
	if connectionID > 0 {
		query = query.Where("connection_id = ?", connectionID)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var result []model.ExternalPrincipalConflict
	err := query.Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&result).Error
	return result, total, err
}

func (r *ExternalPrincipalResolver) ResolveConflict(conflictID, canonicalID, targetUserID, reviewerID uint, note string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var conflict model.ExternalPrincipalConflict
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&conflict, conflictID).Error; err != nil {
			return err
		}
		if conflict.Status != model.PrincipalConflictPending {
			return errors.New("冲突已处理")
		}
		candidate := map[uint]bool{}
		for _, raw := range conflict.CandidatePrincipalIDs {
			id, _ := strconv.ParseUint(raw, 10, 64)
			candidate[uint(id)] = true
		}
		if !candidate[canonicalID] {
			return errors.New("canonical principal 不属于冲突候选")
		}
		var principals []model.ExternalPrincipal
		ids := make([]uint, 0, len(candidate))
		for id := range candidate {
			ids = append(ids, id)
		}
		if err := tx.Unscoped().Where("id IN ?", ids).Find(&principals).Error; err != nil {
			return err
		}
		userSet := map[uint]bool{}
		for _, principal := range principals {
			if principal.UserID != nil {
				userSet[*principal.UserID] = true
			}
		}
		if len(userSet) > 1 && targetUserID == 0 {
			return errors.New("候选 principal 关联不同用户，必须显式选择 target user")
		}
		if targetUserID == 0 {
			for id := range userSet {
				targetUserID = id
			}
		}
		if targetUserID > 0 {
			var target model.User
			if err := tx.First(&target, targetUserID).Error; err != nil {
				return fmt.Errorf("target user 不存在: %w", err)
			}
			userSet[targetUserID] = true
		}
		for _, duplicate := range principals {
			if duplicate.ID == canonicalID {
				continue
			}
			var identifiers []model.ExternalIdentifier
			if err := tx.Where("principal_id = ?", duplicate.ID).Find(&identifiers).Error; err != nil {
				return err
			}
			for _, identifier := range identifiers {
				var count int64
				tx.Model(&model.ExternalIdentifier{}).Where("principal_id = ? AND namespace = ? AND value = ?", canonicalID, identifier.Namespace, identifier.Value).Count(&count)
				if count > 0 {
					if err := tx.Delete(&identifier).Error; err != nil {
						return err
					}
				} else if err := tx.Model(&identifier).Update("principal_id", canonicalID).Error; err != nil {
					return err
				}
			}
			var memberships []model.ExternalMembership
			if err := tx.Where("principal_id = ?", duplicate.ID).Find(&memberships).Error; err != nil {
				return err
			}
			for _, membership := range memberships {
				var existing int64
				tx.Model(&model.ExternalMembership{}).Where("connection_id = ? AND principal_id = ? AND external_org_unit_id = ?", membership.ConnectionID, canonicalID, membership.ExternalOrgUnitID).Count(&existing)
				if existing > 0 {
					if err := tx.Delete(&membership).Error; err != nil {
						return err
					}
				} else if err := tx.Model(&membership).Updates(map[string]any{"principal_id": canonicalID, "user_id": nullableUint(targetUserID)}).Error; err != nil {
					return err
				}
			}
			if err := tx.Delete(&duplicate).Error; err != nil {
				return err
			}
		}
		if targetUserID > 0 {
			if err := tx.Model(&model.ExternalPrincipal{}).Where("id = ?", canonicalID).Updates(map[string]any{"user_id": targetUserID, "status": model.PrincipalStatusActive}).Error; err != nil {
				return err
			}
			if err := tx.Model(&model.ExternalMembership{}).Where("principal_id = ?", canonicalID).Update("user_id", targetUserID).Error; err != nil {
				return err
			}
		}
		now := time.Now()
		for userID := range userSet {
			if err := RevokeLoginSessions(tx, userID, "", "", "identity_changed", &reviewerID); err != nil {
				return err
			}
			if err := tx.Model(&model.User{}).Where("id = ?", userID).Update("session_revoked_at", now).Error; err != nil {
				return err
			}
			if err := dao.NewOIDCTokenDAO(tx).RevokeByUser(userID); err != nil {
				return err
			}
		}
		return tx.Model(&conflict).Updates(map[string]any{"status": model.PrincipalConflictResolved, "canonical_principal_id": canonicalID, "target_user_id": nullableUint(targetUserID), "resolution_note": note, "reviewed_by": reviewerID, "reviewed_at": now}).Error
	})
}

func nullableUint(value uint) any {
	if value == 0 {
		return nil
	}
	return value
}

func (r *ExternalPrincipalResolver) RejectConflict(conflictID, reviewerID uint, note string) error {
	now := time.Now()
	result := r.db.Model(&model.ExternalPrincipalConflict{}).Where("id = ? AND status = ?", conflictID, model.PrincipalConflictPending).Updates(map[string]any{"status": model.PrincipalConflictRejected, "resolution_note": note, "reviewed_by": reviewerID, "reviewed_at": now})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("冲突不存在或已处理")
	}
	return nil
}
