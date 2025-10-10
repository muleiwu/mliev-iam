# Testing Guide for Protocol Abstraction Refactoring

## Prerequisites

Before testing, ensure you have:
1. Go 1.19+ installed
2. MySQL database running
3. Redis running (if used)
4. Node.js 18+ and pnpm installed

## Backend Testing

### 1. Build the Backend

```bash
cd /Users/hepeichun/Code/cnb.cool/mliev/iam/mliev-iam
go build -o iam-server .
```

### 2. Run Database Migration

The migration will:
- Drop the old `oidc_clients` table
- Create the new `applications` table
- Migrate other tables as needed

```bash
# Start the server (it will run migrations automatically)
./iam-server
```

Or if you have a separate migration command:
```bash
go run main.go migrate
```

### 3. Test Application Management API

#### Create an OIDC Application

```bash
curl -X POST http://localhost:8080/api/admin/applications \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_ADMIN_TOKEN" \
  -d '{
    "name": "Test OIDC App",
    "description": "Test OIDC Application",
    "protocolType": "oidc",
    "protocolConfig": {
      "redirectUris": ["http://localhost:3000/callback"],
      "grantTypes": ["authorization_code", "refresh_token"],
      "responseTypes": ["code"],
      "scopes": ["openid", "profile", "email"],
      "clientType": "confidential",
      "requirePkce": false
    }
  }'
```

Expected response:
```json
{
  "code": 200,
  "message": "应用创建成功",
  "data": {
    "id": 1,
    "name": "Test OIDC App",
    "description": "Test OIDC Application",
    "protocolType": "oidc",
    "protocolConfig": {
      "clientId": "generated-uuid",
      "clientSecret": "generated-secret-shown-once",
      "redirectUris": ["http://localhost:3000/callback"],
      ...
    },
    "isActive": true,
    "isVerified": false,
    "createdAt": "2024-01-01T00:00:00Z",
    "updatedAt": "2024-01-01T00:00:00Z"
  }
}
```

#### List Applications

```bash
curl -X GET "http://localhost:8080/api/admin/applications?page=1&pageSize=10" \
  -H "Authorization: Bearer YOUR_ADMIN_TOKEN"
```

#### List Applications by Protocol Type

```bash
curl -X GET "http://localhost:8080/api/admin/applications?page=1&pageSize=10&protocolType=oidc" \
  -H "Authorization: Bearer YOUR_ADMIN_TOKEN"
```

#### Get Application Details

```bash
curl -X GET "http://localhost:8080/api/admin/applications/1" \
  -H "Authorization: Bearer YOUR_ADMIN_TOKEN"
```

#### Update Application

```bash
curl -X PUT http://localhost:8080/api/admin/applications/1 \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_ADMIN_TOKEN" \
  -d '{
    "name": "Updated App Name",
    "isActive": true
  }'
```

#### Delete Application

```bash
curl -X DELETE http://localhost:8080/api/admin/applications/1 \
  -H "Authorization: Bearer YOUR_ADMIN_TOKEN"
```

#### Get Statistics

```bash
curl -X GET http://localhost:8080/api/admin/applications/stats \
  -H "Authorization: Bearer YOUR_ADMIN_TOKEN"
```

Expected response:
```json
{
  "code": 200,
  "data": {
    "activeApplications": 5,
    "inactiveApplications": 2,
    "activeTokens": 0
  }
}
```

### 4. Test OIDC Protocol Endpoints

The public OIDC endpoints should continue to work unchanged:

#### OIDC Discovery Document
```bash
curl http://localhost:8080/.well-known/openid-configuration
```

#### OIDC Authorization
```bash
# Navigate to:
http://localhost:8080/api/oidc/authorize?response_type=code&client_id=YOUR_CLIENT_ID&redirect_uri=YOUR_REDIRECT_URI&scope=openid%20profile&state=random_state
```

#### OIDC Token Exchange
```bash
curl -X POST http://localhost:8080/api/oidc/token \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "grant_type=authorization_code&code=AUTH_CODE&redirect_uri=YOUR_REDIRECT_URI&client_id=YOUR_CLIENT_ID&client_secret=YOUR_CLIENT_SECRET"
```

## Frontend Testing

### 1. Install Dependencies

```bash
cd admin-web/apps/web-antd
pnpm install
```

### 2. Start Development Server

```bash
pnpm dev
```

### 3. Test in Browser

1. Navigate to `http://localhost:5173` (or configured port)
2. Login with admin credentials
3. Navigate to Application Management
4. **Note**: The UI may not fully support protocol selection yet as Vue components need updating

### Expected Behavior (After Vue Updates)

1. **Create Application**: Should show protocol type selector (OIDC, SAML, CAS)
2. **Application List**: Should display protocol type column
3. **Edit Application**: Should show protocol-specific fields
4. **View Application**: Should display protocol type badge

## Common Issues

### Issue 1: Migration Fails
```
Error: table oidc_clients doesn't exist
```
**Solution**: This is expected if starting fresh. The migration will create the new `applications` table.

### Issue 2: Authorization Errors
```
Error: invalid_client
```
**Solution**: Ensure you're using the correct `clientId` and `clientSecret` from the creation response.

### Issue 3: OIDC Endpoints Not Working
```
Error: 应用不存在
```
**Solution**: Make sure you've created at least one OIDC application through the new API.

### Issue 4: Frontend Type Errors
```
TypeScript error: Property 'clientId' does not exist on type 'OIDCConfig | SAMLConfig | CASConfig'
```
**Solution**: Use type guards to check protocol type before accessing protocol-specific fields:
```typescript
if (app.protocolType === 'oidc') {
  const config = app.protocolConfig as ApplicationManagementApi.OIDCConfig;
  console.log(config.clientId);
}
```

## Rollback Plan

If issues occur, you can rollback by:

1. Stop the server
2. Restore database backup (if you created one)
3. Revert code changes:
```bash
git stash  # or git reset --hard HEAD~1
```

## Performance Testing

### Load Test Application Creation

```bash
# Using Apache Bench
ab -n 100 -c 10 -H "Authorization: Bearer YOUR_TOKEN" -p create_app.json -T application/json http://localhost:8080/api/admin/applications
```

### Monitor Database Performance

```sql
-- Check table sizes
SELECT 
    table_name,
    table_rows,
    data_length,
    index_length
FROM information_schema.tables
WHERE table_schema = 'your_database'
AND table_name IN ('applications', 'authorization_codes', 'oidc_tokens');

-- Check slow queries
SHOW FULL PROCESSLIST;
```

## Verification Checklist

- [ ] Backend compiles without errors
- [ ] Database migration completes successfully
- [ ] Can create OIDC application via new API
- [ ] Can list applications
- [ ] Can get application details
- [ ] Can update application
- [ ] Can delete application
- [ ] OIDC authorization flow works
- [ ] OIDC token exchange works
- [ ] OIDC userinfo endpoint works
- [ ] Frontend builds without errors
- [ ] Admin can login
- [ ] Application list loads
- [ ] Statistics display correctly

## Next Steps After Testing

1. Update Vue components for full protocol support
2. Add comprehensive unit tests
3. Add integration tests
4. Update user documentation
5. Plan production deployment

