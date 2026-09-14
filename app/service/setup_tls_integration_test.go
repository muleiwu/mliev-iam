package service

import (
	"context"
	"crypto/x509"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"testing"
	"time"

	dbconfig "cnb.cool/mliev/open/go-web/pkg/server/database/config"
	dbdriver "cnb.cool/mliev/open/go-web/pkg/server/database/driver"
	redisconfig "cnb.cool/mliev/open/go-web/pkg/server/redis/config"
	redisdriver "cnb.cool/mliev/open/go-web/pkg/server/redis/driver"
	setupkit "cnb.cool/mliev/open/setup-kit"
	mysqlcheck "cnb.cool/mliev/open/setup-kit/checkers/mysql"
	pgcheck "cnb.cool/mliev/open/setup-kit/checkers/postgresql"
	redischeck "cnb.cool/mliev/open/setup-kit/checkers/redis"
	"gorm.io/gorm"
)

// Invoked by iam-ee/scripts/setup_tls_acceptance.mjs with isolated native
// servers and a temporary CA. The OS trust store is never modified.
func TestSetupTLSRuntimeConnections(t *testing.T) {
	file := os.Getenv("IAM_SETUP_TLS_FIXTURE")
	if file == "" {
		t.Skip("run the isolated setup TLS acceptance script")
	}
	var fixture struct {
		CA          string
		Connections []setupkit.ConnectionValues
	}
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	pem, err := os.ReadFile(fixture.CA)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(pem) {
		t.Fatal("invalid test CA")
	}
	t.Setenv("GODEBUG", os.Getenv("GODEBUG")+",x509usefallbackroots=1")
	x509.SetFallbackRoots(roots)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	for _, connection := range fixture.Connections {
		t.Run(connection.Driver, func(t *testing.T) {
			v := connection.Values
			var checker setupkit.Checker
			switch connection.Driver {
			case "mysql":
				checker = mysqlcheck.New()
			case "postgresql":
				checker = pgcheck.New()
			case "redis":
				checker = redischeck.New()
			default:
				t.Fatal("unknown fixture driver")
			}
			result, err := checker.Check(ctx, v)
			if err != nil || !result.OK {
				t.Fatalf("TLS checker failed: %+v %v", result, err)
			}
			if connection.Driver == "redis" {
				client, err := redisdriver.RedisFactory(&redisconfig.RedisConfig{Host: v.String("host"), Port: v.Int("port"), Username: v.String("username"), Password: v.String("password"), TLS: true})
				if err != nil {
					t.Fatal("runtime Redis TLS/ACL connection failed")
				}
				defer client.Close()
				if err := client.Set(ctx, "setup-tls-test", "ok", time.Minute).Err(); err != nil {
					t.Fatal(err)
				}
				if value, err := client.Get(ctx, "setup-tls-test").Result(); err != nil || value != "ok" {
					t.Fatal("Redis read/write failed")
				}
			} else {
				if err := withSetupDatabase(ctx, connection, func(db *gorm.DB, _ *sql.DB) error { return db.Exec("SELECT 1").Error }); err != nil {
					t.Fatal("installation TLS connection failed")
				}
				db, err := dbdriver.DatabaseDriverManager.Make(connection.Driver, &dbconfig.DatabaseConfig{Driver: connection.Driver, Host: v.String("host"), Port: v.Int("port"), Username: v.String("username"), Password: v.String("password"), DBName: v.String("database"), TLS: true, Charset: v.String("charset"), PreferSimpleProtocol: true})
				if err != nil {
					t.Fatal("runtime SQL TLS connection failed")
				}
				conn, _ := db.DB()
				defer conn.Close()
				if connection.Driver == "mysql" {
					var charset string
					if err := db.Raw("SELECT @@character_set_client").Scan(&charset).Error; err != nil || (charset != "utf8mb3" && charset != "utf8") {
						t.Fatalf("runtime charset: %s", charset)
					}
				}
			}
			wrong := setupkit.Values{}
			for k, value := range v {
				wrong[k] = value
			}
			wrong["password"] = "deliberately-wrong-test-password"
			result, err = checker.Check(ctx, wrong)
			if err == nil && result.OK {
				t.Fatal("incorrect credentials accepted")
			}
			wrong["password"], wrong["host"] = v["password"], "localhost"
			result, err = checker.Check(ctx, wrong)
			if err == nil && result.OK {
				t.Fatal("hostname absent from certificate accepted")
			}
		})
	}
	untrusted := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer untrusted.Close()
	endpoint, _ := url.Parse(untrusted.URL)
	port, _ := strconv.Atoi(endpoint.Port())
	values := setupkit.Values{"host": endpoint.Hostname(), "port": port, "tls": true, "db": 0}
	result, err := redischeck.New().Check(ctx, values)
	if err == nil && result.OK {
		t.Fatal("untrusted certificate accepted by checker")
	}
	if client, err := redisdriver.RedisFactory(&redisconfig.RedisConfig{Host: endpoint.Hostname(), Port: port, TLS: true}); err == nil {
		client.Close()
		t.Fatal("untrusted certificate accepted by runtime")
	}
}
