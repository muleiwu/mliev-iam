# 身份凭据主密钥使用与轮换指南

> 适用范围：`IDENTITY_CREDENTIALS_KEYS`、`IDENTITY_CREDENTIALS_ACTIVE_KEY_VERSION`，以及兼容变量 `IDENTITY_CREDENTIALS_KEY`。
>
> 这套密钥用于身份连接凭据和外部事件 payload 的应用层加密，不是用户密码哈希、数据库密码、Session 密钥或 OIDC/JWT 应用签名密钥。

## 1. 目标与安全边界

身份凭据主密钥是部署级 AES-256-GCM 主密钥。应用从运行环境读取密钥，但不会把主密钥写入数据库。数据库只保存密文、随机 nonce、算法和密钥版本。

它提供以下保证：

- 数据库单独泄漏时，攻击者不能直接读取第三方平台凭据和事件 payload。
- AES-GCM 同时提供机密性与完整性，错误密钥或被篡改的密文无法通过解密校验。
- AAD 使用 `connection_id:purpose:key_version`，防止密文被移动到其他连接、用途或版本后继续解密。
- 密钥环允许新旧版本并存，使滚动发布和渐进式重加密成为可能。

它不保护数据库中的所有个人信息。用户、部门、外部 principal、同步快照和任务元数据等字段仍按各自表结构保存。

## 2. 配置格式

### 2.1 推荐：版本化密钥环

```text
IDENTITY_CREDENTIALS_KEYS={"1":"<Base64 编码的 32 字节密钥>"}
IDENTITY_CREDENTIALS_ACTIVE_KEY_VERSION=1
```

约束：

- `IDENTITY_CREDENTIALS_KEYS` 是 `version -> key` 的 JSON 对象，也兼容 `1:key,2:key` 格式。
- 每个版本必须是正整数。
- 每个 key 必须是 32 字节随机数据的标准 Base64 编码。
- 活动版本必须存在于密钥环中。
- 新数据只使用活动版本加密；解密根据每条记录保存的版本选择密钥。

### 2.2 兼容：单密钥变量

```text
IDENTITY_CREDENTIALS_KEY=<Base64 编码的 32 字节密钥>
```

该变量只表示 version 1，用于兼容旧部署。新部署应优先使用版本化密钥环，避免以后轮换时改变配置模型。

如果同时存在 `IDENTITY_CREDENTIALS_KEYS` 和 `IDENTITY_CREDENTIALS_KEY`，应用优先使用前者。

## 3. 受保护的数据

### 3.1 `identity_connection_credentials`

`ciphertext` 保存完整凭据 JSON 的密文。凭据按 `purpose` 分为：

| `purpose` | 典型内容 |
| --- | --- |
| `authentication` | `clientId`、`clientSecret` |
| `directory` | `appId`、`appKey`、`appSecret`、`corpId`、`directorySecret`、`tenantKey`、`rootDepartmentId` |
| `events` | `verificationToken`、`encryptKey`、`eventToken`、`token`、`encodingAesKey`、`corpId` |

以下列是密文或解密元数据：

| 列 | 是否保密 | 说明 |
| --- | --- | --- |
| `ciphertext` | 是 | Base64 编码的 AES-GCM 密文 |
| `nonce` | 否 | 每次加密重新生成的随机 nonce |
| `algorithm` | 否 | 当前为 `AES-256-GCM` |
| `key_version` | 否 | 指定解密所需的密钥版本 |
| `connection_id`、`purpose` | 否 | 同时参与 AAD 绑定 |

凭据保存和读取实现位于 [`app/service/identity_connection_service.go`](../app/service/identity_connection_service.go)，加密实现位于 [`internal/pkg/credentialcrypto/cipher.go`](../internal/pkg/credentialcrypto/cipher.go)。

### 3.2 `external_event_inbox`

`payload_ciphertext` 保存飞书、Lark、企业微信、钉钉等平台回调事件解析后 payload JSON 的密文。

| 列 | 是否保密 | 说明 |
| --- | --- | --- |
| `payload_ciphertext` | 是 | Base64 编码的事件 payload 密文 |
| `payload_nonce` | 否 | 该事件的随机 nonce |
| `payload_algorithm` | 否 | 当前为 `AES-256-GCM` |
| `payload_key_version` | 否 | 指定解密所需的密钥版本 |

`provider_event_id`、`event_type`、`object_type`、`external_object_id`、状态和时间等事件元数据不使用这把密钥加密。

事件加解密实现位于 [`app/service/external_event_service.go`](../app/service/external_event_service.go)。

## 4. 首次配置

### 4.1 判断能否生成新密钥

只有在确认从未使用过该加密功能，或两张表均无加密数据时，才能生成全新的 version 1 密钥。

PostgreSQL 可先检查表和版本分布：

```sql
SELECT
  to_regclass('public.identity_connection_credentials'),
  to_regclass('public.external_event_inbox');
```

表存在时继续检查：

```sql
SELECT key_version, COUNT(*)
FROM identity_connection_credentials
GROUP BY key_version
ORDER BY key_version;

SELECT payload_key_version, COUNT(*)
FROM external_event_inbox
GROUP BY payload_key_version
ORDER BY payload_key_version;
```

只要存在加密记录，就必须恢复对应版本的原密钥，不能用新随机密钥替代。

### 4.2 创建 Kubernetes Secret

下面示例不会把密钥作为命令字面量写入 Shell 历史。命令执行期间不要启用 `set -x`，也不要打印变量。

```bash
deploy_namespace="mliev-iam"
credential_key="$(openssl rand -base64 32)"
credential_keys_json="$(printf '{"1":"%s"}' "$credential_key")"
kubectl create secret generic mliev-iam-credential-keys -n "$deploy_namespace" --from-literal=IDENTITY_CREDENTIALS_KEYS="$credential_keys_json" --from-literal=IDENTITY_CREDENTIALS_ACTIVE_KEY_VERSION="1"
unset credential_key credential_keys_json
```

安全检查字段名称和大小，不输出密钥：

```bash
kubectl describe secret mliev-iam-credential-keys -n mliev-iam
```

验证 version 1 解码后是 32 字节：

```bash
kubectl get secret mliev-iam-credential-keys -n mliev-iam -o jsonpath='{.data.IDENTITY_CREDENTIALS_KEYS}' | base64 -d | python3 -c 'import base64,json,sys; data=json.load(sys.stdin); print(len(base64.b64decode(data["1"], validate=True)))'
```

预期输出为 `32`。

### 4.3 注入 Deployment

```bash
kubectl set env deployment/mliev-iam -n mliev-iam --from=secret/mliev-iam-credential-keys
kubectl rollout status deployment/mliev-iam -n mliev-iam
```

生产环境应在 Deployment/Helm/ExternalSecret 配置中声明稳定的 `secretKeyRef` 或 `envFrom`，而不是只依赖一次性的手工命令。

## 5. 启动行为

迁移运行器在执行任何待应用迁移前检查密钥配置。缺失、Base64 无效、长度错误、活动版本缺失都会阻止迁移。

迁移完成后，应用会遍历 `identity_connection_credentials` 并尝试解密全部凭据。任何记录不可解密都会导致迁移服务报错。

当前启动校验不会遍历 `external_event_inbox` 的全部历史 payload。因此轮换和删除旧密钥前，必须显式检查 `payload_key_version` 分布。

实现位于 [`migrations/runner.go`](../migrations/runner.go)。

## 6. 轮换支持现状

| 能力 | 状态 |
| --- | --- |
| 多版本密钥同时加载 | 支持 |
| 新数据使用活动版本加密 | 支持 |
| 根据记录版本解密旧数据 | 支持 |
| 身份凭据启动解密校验 | 支持 |
| 保存凭据时自然升级到活动版本 | 支持 |
| 存量凭据批量重加密 | 未实现 |
| 历史事件 payload 批量重加密 | 未实现 |
| 管理后台一键轮换凭据主密钥 | 未实现 |
| 自动判断并删除无引用旧密钥 | 未实现 |

管理后台现有的“轮转应用密钥”是 OIDC/JWT 应用签名密钥轮转，与本文的身份凭据主密钥无关。

## 7. 安全轮换流程

以下以 version 1 轮换到 version 2 为例。禁止直接用只有 version 2 的密钥环替换 version 1。

### 7.1 轮换前检查

1. 备份数据库。
2. 确认当前 Secret 的 version 1 已在外部密码库或密钥管理系统中备份。
3. 查询两张表的版本分布并记录基线。
4. 确认所有应用副本引用同一个 Secret。
5. 确认发布期间不会同时使用未包含 version 2 的旧镜像配置启动新副本。

### 7.2 阶段 A：分发新密钥但保持 version 1 活动

目标配置：

```text
IDENTITY_CREDENTIALS_KEYS={"1":"<旧密钥>","2":"<新密钥>"}
IDENTITY_CREDENTIALS_ACTIVE_KEY_VERSION=1
```

完成 Secret 更新后滚动重启全部 Pod：

```bash
kubectl rollout restart deployment/mliev-iam -n mliev-iam
kubectl rollout status deployment/mliev-iam -n mliev-iam
```

此阶段所有 Pod 都必须能够加载 version 1 和 version 2，但仍只写 version 1。这样下一阶段滚动发布期间，尚未重启的旧 Pod 也能解密新 Pod 写入的 version 2 数据。

### 7.3 阶段 B：切换活动版本到 version 2

目标配置：

```text
IDENTITY_CREDENTIALS_KEYS={"1":"<旧密钥>","2":"<新密钥>"}
IDENTITY_CREDENTIALS_ACTIVE_KEY_VERSION=2
```

再次滚动重启：

```bash
kubectl rollout restart deployment/mliev-iam -n mliev-iam
kubectl rollout status deployment/mliev-iam -n mliev-iam
```

此后所有新凭据和新事件使用 version 2。version 1 数据仍可正常读取。

仅更新 Kubernetes Secret 不会刷新已运行容器的环境变量，必须触发 Pod 重建。

### 7.4 迁移存量密文

`identity_connection_credentials` 在管理员重新保存凭据时会：

1. 按旧 `key_version` 解密；
2. 合并未修改的秘密字段；
3. 使用当前活动版本和新 nonce 重新加密；
4. 更新 `key_version` 和 `rotated_at`。

当前没有批量重加密命令。连接较少时可以逐个重新保存 `authentication`、`directory`、`events` 凭据；连接较多时应实现专用批处理任务。

`external_event_inbox` 历史记录不会自动重加密。可选择：

- 按保留策略删除过期事件；
- 长期保留旧密钥；
- 实现批量重加密任务。

批量任务必须对每条记录执行“旧版本解密 → 活动版本加密 → 原子更新密文、nonce、算法、版本”。由于 AAD 包含版本号，禁止只修改 `key_version` 或 `payload_key_version`。

### 7.5 删除旧密钥前的硬门槛

以下查询都必须返回 `0`：

```sql
SELECT COUNT(*) AS credentials_using_v1
FROM identity_connection_credentials
WHERE key_version = 1;

SELECT COUNT(*) AS events_using_v1
FROM external_event_inbox
WHERE payload_key_version = 1;
```

确认所有备份、延迟任务、离线副本和回滚镜像也不再依赖 version 1 后，才能把密钥环改为：

```text
IDENTITY_CREDENTIALS_KEYS={"2":"<新密钥>"}
IDENTITY_CREDENTIALS_ACTIVE_KEY_VERSION=2
```

删除 version 1 后再次滚动重启，并执行启动、登录、目录同步和事件回调验证。

## 8. 轮换后的验证

### 8.1 Kubernetes

```bash
kubectl rollout status deployment/mliev-iam -n mliev-iam
kubectl get pods -n mliev-iam
kubectl logs deployment/mliev-iam -n mliev-iam --tail=200
```

不得出现：

```text
凭据主密钥检查失败
凭据密钥版本 vN 未配置
凭据解密失败
启动前身份连接凭据解密验证失败
```

### 8.2 数据库

```sql
SELECT key_version, COUNT(*)
FROM identity_connection_credentials
GROUP BY key_version
ORDER BY key_version;

SELECT payload_key_version, COUNT(*)
FROM external_event_inbox
GROUP BY payload_key_version
ORDER BY payload_key_version;
```

切换活动版本后新写入记录应逐步出现在新版本下。

### 8.3 业务验收

- 使用至少一个社会化或企业身份连接完成登录。
- 对每种已启用 provider 执行连接健康检查。
- 执行一次目录同步预览或诊断。
- 发送并处理一次外部事件回调。
- 确认管理 API 只返回 `configured`、`keyVersion`、`rotatedAt` 等摘要，不返回明文或密文。

## 9. 回滚与故障恢复

### 9.1 切换 version 2 后需要回滚

可以把活动版本暂时改回 version 1，但密钥环必须继续包含 version 1 和 version 2。因为切换期间可能已经产生 version 2 数据，删除 version 2 会使这些记录无法解密。

安全回滚配置：

```text
IDENTITY_CREDENTIALS_KEYS={"1":"<旧密钥>","2":"<新密钥>"}
IDENTITY_CREDENTIALS_ACTIVE_KEY_VERSION=1
```

### 9.2 密钥丢失

- 数据库不能反推出 AES 主密钥。
- `identity_connection_credentials` 无法解密时，只能重新录入对应第三方凭据。
- `external_event_inbox` 无法解密时，历史事件 payload 无法恢复；可按审计和保留策略删除。
- 如果旧版迁移尚未开始，旧表中的明文凭据仍可能存在，应在迁移事务和备份中核实，不能假设已经丢失。

### 9.3 Secret 被错误覆盖

Kubernetes 通常不保留 Secret 历史内容。优先从以下位置恢复：

1. 外部 Secret Manager、Vault 或密码库；
2. GitOps 加密 Secret，例如 SOPS/Sealed Secrets；
3. 集群或 etcd 备份；
4. 尚未重启、仍持有旧环境变量的 Pod。

不要把恢复出的密钥粘贴到工单、聊天、普通日志或 Git 仓库。

## 10. 运维要求

- 所有副本必须使用同一个密钥环和活动版本。
- 禁止为每次部署重新生成密钥。
- 禁止把真实密钥写入镜像、仓库、ConfigMap 或普通配置文件。
- Secret 更新和轮换操作期间禁止启用 Shell `set -x`。
- 日志不得输出 `IDENTITY_CREDENTIALS_KEY(S)` 的值、解密后的凭据或事件 payload。
- 密钥环变更必须记录版本、时间、操作者、变更原因和验证结果，但审计记录中不得包含密钥值。
- 删除旧版本前必须以数据库版本计数为准，不能只根据轮换时间判断。
