package service

import (
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"cnb.cool/mliev/iam/mliev-iam/app/service/protocol"
	"cnb.cool/mliev/iam/mliev-iam/pkg/firstparty"
	"context"
	"testing"
)

func TestExtensionClientSecurityAndAuthNamespace(t *testing.T) {
	client := firstparty.Client{ID: "ce84792f-38e6-4474-855d-316a4f431acc", Name: "Test workspace", CallbackPath: "/api/workspace/auth/callback"}
	firstparty.Register(client)
	db := newSystemApplicationTestDB(t)
	app, err := EnsureFirstPartyApplication(db, client.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	again, err := EnsureFirstPartyApplication(db, client.ID, nil)
	if err != nil || again.ID != app.ID {
		t.Fatal("not idempotent")
	}
	if ApplicationAllowsRegistration(app) || !app.IsProtected || !app.IsActive {
		t.Fatal("invalid built-in policy")
	}
	handler := protocol.NewOIDCProtocolHandler(db)
	issuer := "https://iam.example/oidc/" + client.ID
	if err := handler.ValidateRedirectURI(app, "https://iam.example"+client.CallbackPath, issuer); err != nil {
		t.Fatal(err)
	}
	for _, u := range []string{"https://elsewhere.example" + client.CallbackPath, "https://iam.example/api/admin/auth/callback", "https://iam.example" + client.CallbackPath + "?x=1"} {
		if handler.ValidateRedirectURI(app, u, issuer) == nil {
			t.Fatal("unsafe redirect", u)
		}
	}
	if _, err := EnsureFirstPartyApplication(db, "unregistered", nil); err == nil {
		t.Fatal("accepted unregistered client")
	}
	if !ApplicationAllowsRegistration(&model.Application{UUID: "other", IsProtected: true}) {
		t.Fatal("changed ordinary protected app")
	}
	store := &memoryAdminAuthStore{}
	admin := NewAdminAuthFlowService(nil)
	extension := NewFirstPartyAuthFlowService(nil, "test-workspace")
	admin.store = store
	extension.store = store
	txn, binding, err := extension.Start(context.Background(), "https://iam.example"+client.CallbackPath, "/workspace/")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Consume(context.Background(), txn.State, binding); err == nil {
		t.Fatal("admin consumed extension transaction")
	}
	if _, err := extension.Consume(context.Background(), txn.State, binding); err != nil {
		t.Fatal(err)
	}
}
