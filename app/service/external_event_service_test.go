package service

import (
	"context"
	"encoding/base64"
	"net/http"
	"testing"

	"cnb.cool/mliev/iam/mliev-iam/app/model"
	eventprotocol "cnb.cool/mliev/iam/mliev-iam/app/service/protocol/event"
	"cnb.cool/mliev/iam/mliev-iam/internal/pkg/credentialcrypto"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type fixedEventAdapter struct{ event *eventprotocol.DirectoryEvent }

func (a fixedEventAdapter) VerifyAndParse(context.Context, http.Header, []byte) (*eventprotocol.DirectoryEvent, error) {
	return a.event, nil
}

func TestExternalEventInboxIsEncryptedAndIdempotent(t *testing.T) {
	t.Setenv(credentialcrypto.EnvironmentKey, base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef")))
	db, err := gorm.Open(sqlite.Open("file:event-inbox?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.IdentityConnection{}, &model.IdentityConnectionCredential{}, &model.Department{}, &model.DirectorySyncPolicy{}, &model.DirectorySyncRun{}, &model.ExternalEventInbox{}, &model.SyncJob{}))
	connection := &model.IdentityConnection{Name: "飞书事件", Identifier: "feishu-events", Category: model.ConnectionCategoryEnterprise, ProviderType: "feishu", Capabilities: model.StringList{"directory_sync", "events"}, Status: model.ConnectionStatusEnabled}
	require.NoError(t, db.Create(connection).Error)
	root := &model.Department{Name: "事件同步根", Code: "event-root", Status: model.DepartmentStatusEnabled}
	require.NoError(t, db.Create(root).Error)
	require.NoError(t, db.Create(&model.DirectorySyncPolicy{ConnectionID: connection.ID, RootDepartmentID: root.ID, MatchPolicy: model.DirectoryMatchManual, DeprovisionPolicy: model.DirectoryDeprovisionBlockIdentityRequireApproval, Enabled: true}).Error)
	require.NoError(t, NewIdentityConnectionService(db).SaveCredentials(connection.ID, model.CredentialPurposeEvents, map[string]any{"verificationToken": "token"}))
	factory := func(*model.IdentityConnection, map[string]any) (eventprotocol.Adapter, error) {
		return fixedEventAdapter{event: &eventprotocol.DirectoryEvent{ProviderEventID: "evt-idempotent", EventType: "contact.user.updated_v3", ObjectType: "user", ExternalObjectID: "u-1", Payload: map[string]any{"email": "secret@example.com"}}}, nil
	}
	service := NewExternalEventServiceWithFactory(db, factory)
	first, err := service.Receive(context.Background(), connection.Identifier, nil, []byte("signed-body"))
	require.NoError(t, err)
	require.False(t, first.Duplicate)
	second, err := service.Receive(context.Background(), connection.Identifier, nil, []byte("signed-body"))
	require.NoError(t, err)
	require.True(t, second.Duplicate)
	var inboxCount, jobCount int64
	require.NoError(t, db.Model(&model.ExternalEventInbox{}).Count(&inboxCount).Error)
	require.NoError(t, db.Model(&model.SyncJob{}).Count(&jobCount).Error)
	require.EqualValues(t, 1, inboxCount)
	require.EqualValues(t, 1, jobCount)
	require.NotContains(t, first.Inbox.PayloadCiphertext, "secret@example.com")
	payload, err := service.DecryptPayload(first.Inbox.ID)
	require.NoError(t, err)
	require.Equal(t, "secret@example.com", payload["email"])
}
