package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/model"
	eventprotocol "cnb.cool/mliev/iam/mliev-iam/app/service/protocol/event"
	"cnb.cool/mliev/iam/mliev-iam/internal/pkg/credentialcrypto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type EventAdapterFactory func(*model.IdentityConnection, map[string]any) (eventprotocol.Adapter, error)

type EventReceiveResult struct {
	Challenge           string                    `json:"challenge,omitempty"`
	Duplicate           bool                      `json:"duplicate"`
	Inbox               *model.ExternalEventInbox `json:"inbox,omitempty"`
	ResponseBody        []byte                    `json:"-"`
	ResponseContentType string                    `json:"-"`
}

type ExternalEventListFilter struct {
	ConnectionID uint
	Status       model.ExternalEventStatus
	EventType    string
	Page         int
	PageSize     int
}

type ExternalEventService struct {
	db      *gorm.DB
	factory EventAdapterFactory
}

func NewExternalEventService(db *gorm.DB) *ExternalEventService {
	return &ExternalEventService{db: db, factory: eventprotocol.NewAdapter}
}

func NewExternalEventServiceWithFactory(db *gorm.DB, factory EventAdapterFactory) *ExternalEventService {
	service := NewExternalEventService(db)
	service.factory = factory
	return service
}

func (s *ExternalEventService) Receive(ctx context.Context, identifier string, headers http.Header, body []byte) (*EventReceiveResult, error) {
	connection, err := NewIdentityConnectionService(s.db).GetByIdentifier(identifier)
	if err != nil {
		return nil, fmt.Errorf("事件连接不存在: %w", err)
	}
	if connection.Category != model.ConnectionCategoryEnterprise || connection.Status != model.ConnectionStatusEnabled {
		return nil, fmt.Errorf("事件连接未启用或不是企业连接")
	}
	if !connection.HasCapability(model.ConnectionCapabilityEvents) {
		return nil, fmt.Errorf("连接未启用 events 能力")
	}
	credentials, err := NewIdentityConnectionService(s.db).CredentialValues(connection.ID, model.CredentialPurposeEvents)
	if err != nil {
		return nil, fmt.Errorf("读取事件凭据失败: %w", err)
	}
	adapter, err := s.factory(connection, credentials)
	if err != nil {
		return nil, err
	}
	event, err := adapter.VerifyAndParse(ctx, headers, body)
	if err != nil {
		return nil, err
	}
	if event.Challenge != "" {
		result := &EventReceiveResult{Challenge: event.Challenge}
		if event.CallbackResponse != nil {
			result.ResponseBody = event.CallbackResponse.Body
			result.ResponseContentType = event.CallbackResponse.ContentType
		}
		return result, nil
	}
	payload, err := json.Marshal(event.Payload)
	if err != nil {
		return nil, err
	}
	cipher, err := credentialcrypto.FromEnvironment()
	if err != nil {
		return nil, err
	}
	aadPurpose := "external_event:" + event.ProviderEventID
	envelope, err := cipher.Encrypt(payload, credentialcrypto.AAD(connection.ID, aadPurpose, cipher.ActiveKeyVersion()))
	if err != nil {
		return nil, err
	}
	inbox := &model.ExternalEventInbox{
		ConnectionID: connection.ID, ProviderEventID: event.ProviderEventID,
		EventType: event.EventType, ObjectType: event.ObjectType, ExternalObjectID: event.ExternalObjectID,
		Status: model.ExternalEventVerified, SignatureValid: true,
		PayloadCiphertext: envelope.Ciphertext, PayloadNonce: envelope.Nonce,
		PayloadAlgorithm: envelope.Algorithm, PayloadKeyVersion: envelope.KeyVersion, ReceivedAt: time.Now(),
	}
	duplicate := false
	err = s.db.Transaction(func(tx *gorm.DB) error {
		result := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "connection_id"}, {Name: "provider_event_id"}}, DoNothing: true}).Create(inbox)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			duplicate = true
			return tx.Where("connection_id = ? AND provider_event_id = ?", connection.ID, event.ProviderEventID).First(inbox).Error
		}
		if _, err := NewSyncJobService(tx).Enqueue(connection.ID, model.SyncJobEventRefresh, map[string]any{
			"eventId": inbox.ID, "objectType": event.ObjectType, "externalObjectId": event.ExternalObjectID,
		}, time.Now(), &inbox.ID); err != nil {
			return err
		}
		inbox.Status = model.ExternalEventQueued
		return tx.Model(inbox).Update("status", inbox.Status).Error
	})
	if err != nil {
		return nil, err
	}
	result := &EventReceiveResult{Duplicate: duplicate, Inbox: inbox}
	if event.CallbackResponse != nil {
		result.ResponseBody = event.CallbackResponse.Body
		result.ResponseContentType = event.CallbackResponse.ContentType
	}
	return result, nil
}

func (s *ExternalEventService) DecryptPayload(inboxID uint) (map[string]any, error) {
	var inbox model.ExternalEventInbox
	if err := s.db.First(&inbox, inboxID).Error; err != nil {
		return nil, err
	}
	cipher, err := credentialcrypto.FromEnvironment()
	if err != nil {
		return nil, err
	}
	plain, err := cipher.Decrypt(credentialcrypto.Envelope{
		Ciphertext: inbox.PayloadCiphertext, Nonce: inbox.PayloadNonce,
		Algorithm: inbox.PayloadAlgorithm, KeyVersion: inbox.PayloadKeyVersion,
	}, credentialcrypto.AAD(inbox.ConnectionID, "external_event:"+inbox.ProviderEventID, inbox.PayloadKeyVersion))
	if err != nil {
		return nil, err
	}
	var payload map[string]any
	if err := json.Unmarshal(plain, &payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func (s *ExternalEventService) List(filter ExternalEventListFilter) ([]model.ExternalEventInbox, int64, error) {
	query := s.db.Model(&model.ExternalEventInbox{}).Preload("Connection")
	if filter.ConnectionID != 0 {
		query = query.Where("connection_id = ?", filter.ConnectionID)
	}
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	if filter.EventType != "" {
		query = query.Where("event_type = ?", filter.EventType)
	}
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PageSize < 1 || filter.PageSize > 200 {
		filter.PageSize = 20
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var events []model.ExternalEventInbox
	err := query.Order("id DESC").Offset((filter.Page - 1) * filter.PageSize).Limit(filter.PageSize).Find(&events).Error
	return events, total, err
}

func (s *ExternalEventService) Replay(eventID uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var inbox model.ExternalEventInbox
		if err := tx.First(&inbox, eventID).Error; err != nil {
			return err
		}
		var job model.SyncJob
		err := tx.Where("inbox_event_id = ?", eventID).First(&job).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if _, err := NewSyncJobService(tx).Enqueue(inbox.ConnectionID, model.SyncJobEventRefresh, map[string]any{
				"eventId": inbox.ID, "objectType": inbox.ObjectType, "externalObjectId": inbox.ExternalObjectID,
			}, time.Now(), &inbox.ID); err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else if err := NewSyncJobService(tx).Replay(job.ID); err != nil {
			return err
		}
		return tx.Model(&inbox).Updates(map[string]any{
			"status": model.ExternalEventQueued, "attempts": gorm.Expr("attempts + 1"),
			"last_error": "", "processed_at": nil,
		}).Error
	})
}

func (s *ExternalEventService) MarkProcessed(eventID uint) error {
	now := time.Now()
	return s.db.Model(&model.ExternalEventInbox{}).Where("id = ?", eventID).Updates(map[string]any{"status": model.ExternalEventProcessed, "processed_at": now, "last_error": ""}).Error
}

func (s *ExternalEventService) MarkFailed(eventID uint, dead bool, cause error) error {
	status := model.ExternalEventFailed
	if dead {
		status = model.ExternalEventDeadLetter
	}
	return s.db.Model(&model.ExternalEventInbox{}).Where("id = ?", eventID).Updates(map[string]any{"status": status, "last_error": cause.Error()}).Error
}
