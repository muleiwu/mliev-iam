//go:build sandbox

package directory

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// Real enterprise acceptance gate. Credentials are injected by the controlled
// CI environment and are never printed. A missing variable is a failure (not a
// skip) because phase 04 cannot be marked complete without all three tenants.
//
//	go test -tags=sandbox ./app/service/protocol/directory -run Sandbox -count=1
func TestFeishuSandboxDirectory(t *testing.T) {
	adapter, err := NewFeishuAdapter(map[string]any{
		"appId":     sandboxEnv(t, "FEISHU_SANDBOX_APP_ID"),
		"appSecret": sandboxEnv(t, "FEISHU_SANDBOX_APP_SECRET"),
		"tenantKey": sandboxEnv(t, "FEISHU_SANDBOX_TENANT_KEY"),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	runSandboxDirectoryAcceptance(t, adapter, SyncScope{
		RootExternalOrgUnitIDs: []string{sandboxEnv(t, "FEISHU_SANDBOX_ROOT_DEPARTMENT_ID")},
		IncludeDescendants:     true, IncludeInactive: true,
	})
}

func TestWeComSandboxDirectory(t *testing.T) {
	rootID := sandboxEnv(t, "WECOM_SANDBOX_ROOT_DEPARTMENT_ID")
	adapter, err := NewWeComAdapter(map[string]any{
		"corpId":           sandboxEnv(t, "WECOM_SANDBOX_CORP_ID"),
		"directorySecret":  sandboxEnv(t, "WECOM_SANDBOX_DIRECTORY_SECRET"),
		"rootDepartmentId": rootID,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	runSandboxDirectoryAcceptance(t, adapter, SyncScope{IncludeDescendants: true, IncludeInactive: true})
}

func TestDingTalkSandboxDirectory(t *testing.T) {
	rootID := sandboxEnv(t, "DINGTALK_SANDBOX_ROOT_DEPARTMENT_ID")
	adapter, err := NewDingTalkAdapter(map[string]any{
		"appKey":           sandboxEnv(t, "DINGTALK_SANDBOX_APP_KEY"),
		"appSecret":        sandboxEnv(t, "DINGTALK_SANDBOX_APP_SECRET"),
		"corpId":           sandboxEnv(t, "DINGTALK_SANDBOX_CORP_ID"),
		"rootDepartmentId": rootID,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	runSandboxDirectoryAcceptance(t, adapter, SyncScope{IncludeDescendants: true, IncludeInactive: true})
}

func runSandboxDirectoryAcceptance(t *testing.T, adapter Adapter, scope SyncScope) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	diagnostic, err := adapter.ValidateDirectoryConfig(ctx)
	if err != nil {
		t.Fatalf("凭据/权限诊断失败: %v", err)
	}
	if !diagnostic.TokenValid || !diagnostic.DirectoryReadable || !diagnostic.PaginationComplete {
		t.Fatalf("目录诊断未通过: token=%v readable=%v pagination=%v warnings=%v", diagnostic.TokenValid, diagnostic.DirectoryReadable, diagnostic.PaginationComplete, diagnostic.Warnings)
	}
	reader, ok := adapter.(SnapshotReader)
	if !ok {
		t.Fatal("平台适配器未实现完整快照接口")
	}
	snapshot, err := reader.ReadSnapshot(ctx, scope)
	if err != nil {
		t.Fatalf("完整快照失败: %v", err)
	}
	if snapshot == nil || !snapshot.Completeness.PaginationComplete || snapshot.Completeness.ScopeHash == "" {
		t.Fatalf("快照完整性失败: %#v", snapshotCompletenessForSandbox(snapshot))
	}
	if len(snapshot.OrgUnits) == 0 || len(snapshot.Users) == 0 {
		t.Fatalf("测试企业必须至少包含一个部门和一个用户: departments=%d users=%d", len(snapshot.OrgUnits), len(snapshot.Users))
	}
	orgs := make(map[string]struct{}, len(snapshot.OrgUnits))
	for _, org := range snapshot.OrgUnits {
		if org.ExternalID == "" || strings.TrimSpace(org.Name) == "" {
			t.Fatal("平台返回了缺少稳定 ID 或名称的部门")
		}
		orgs[org.ExternalID] = struct{}{}
	}
	for _, user := range snapshot.Users {
		if user.StableID == "" || user.ProviderUserID == "" || strings.TrimSpace(user.DisplayName) == "" {
			t.Fatal("平台返回了缺少稳定 ID、provider user ID 或名称的用户")
		}
		if !hasSandboxIdentifier(user.Identifiers, "directory_user_id") || !hasSandboxIdentifier(user.Identifiers, "login_subject") {
			t.Fatal("目录用户缺少 directory_user_id/login_subject 完整标识集合")
		}
		for _, membership := range user.Memberships {
			if _, exists := orgs[membership.OrgUnitExternalID]; !exists {
				t.Fatal("用户成员关系引用了快照范围外部门")
			}
		}
	}
	t.Logf("sandbox acceptance passed: departments=%d users=%d scopeHash=%s", len(snapshot.OrgUnits), len(snapshot.Users), snapshot.Completeness.ScopeHash)
}

func sandboxEnv(t *testing.T, name string) string {
	t.Helper()
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		t.Fatalf("缺少真实测试企业环境变量 %s", name)
	}
	return value
}

func hasSandboxIdentifier(identifiers []Identifier, namespace string) bool {
	for _, identifier := range identifiers {
		if identifier.Namespace == namespace && identifier.Value != "" {
			return true
		}
	}
	return false
}

func snapshotCompletenessForSandbox(snapshot *Snapshot) SnapshotCompleteness {
	if snapshot == nil {
		return SnapshotCompleteness{}
	}
	return snapshot.Completeness
}
