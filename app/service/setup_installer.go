package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"embed"
	"encoding/base64"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/utils"
	"cnb.cool/mliev/iam/mliev-iam/migrations"
	"cnb.cool/mliev/iam/mliev-iam/pkg/installation"
	dbConfig "cnb.cool/mliev/open/go-web/pkg/server/database/config"
	"cnb.cool/mliev/open/go-web/pkg/server/database/sqlite"
	httpInterfaces "cnb.cool/mliev/open/go-web/pkg/server/http_server/interfaces"
	"cnb.cool/mliev/open/go-web/pkg/server/reload"
	setupkit "cnb.cool/mliev/open/setup-kit"
	mysqlcheck "cnb.cool/mliev/open/setup-kit/checkers/mysql"
	pgcheck "cnb.cool/mliev/open/setup-kit/checkers/postgresql"
	redischeck "cnb.cool/mliev/open/setup-kit/checkers/redis"
	sqlitecheck "cnb.cool/mliev/open/setup-kit/checkers/sqlite"
	"cnb.cool/mliev/open/setup-kit/presets"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/spf13/viper"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

//go:embed setup_enter.html
var setupEnterHTML []byte

var setupEnterTemplate = template.Must(template.New("setup-enter").Parse(string(setupEnterHTML)))

//go:embed setup_enter.js
var setupEnterJS []byte

//go:embed setup_logo.svg
var setupLogo []byte

//go:embed setup_favicon.ico
var setupFavicon []byte

//go:embed setup_license.txt
var setupLicenseFS embed.FS

type SetupInstaller struct {
	Kit       *setupkit.Kit
	ready     func() bool
	entryPath string
	enterHTML []byte
}

// SetupOptions customizes presentation without duplicating installation logic.
type SetupOptions struct {
	Name      string
	EntryPath string
}

// MySQL and MariaDB have separate form identities but share connection checks.
type setupMySQLChecker struct {
	setupkit.Checker
	id, title string
}

func (c setupMySQLChecker) Descriptor() setupkit.Driver {
	driver := c.Checker.Descriptor()
	driver.ID, driver.Title = c.id, c.title
	return driver
}

func NewSetupInstaller(edition string, installed bool, ready func() bool, options ...SetupOptions) (*SetupInstaller, error) {
	if len(options) > 1 {
		return nil, fmt.Errorf("provide at most one setup options value")
	}
	var presentation SetupOptions
	if len(options) == 1 {
		presentation = options[0]
	}
	if presentation.Name == "" {
		presentation.Name = "MLiev IAM"
		if edition == "enterprise" {
			presentation.Name += " Enterprise"
		}
	}
	if presentation.EntryPath == "" {
		presentation.EntryPath = "/admin/"
	}
	entry, err := url.Parse(presentation.EntryPath)
	if err != nil || entry.IsAbs() || entry.Host != "" || !strings.HasPrefix(entry.Path, "/") || strings.HasPrefix(entry.Path, "//") || strings.ContainsAny(entry.Path, "\\\r\n") || strings.ContainsAny(presentation.EntryPath, "\\\r\n") {
		return nil, fmt.Errorf("setup entry must be a site-relative path")
	}
	var enterHTML bytes.Buffer
	if err := setupEnterTemplate.Execute(&enterHTML, presentation); err != nil {
		return nil, err
	}
	configPath := installation.ConfigPath()
	if !installed {
		if err := os.MkdirAll(filepath.Dir(configPath), 0700); err != nil {
			return nil, err
		}
	}
	db := presets.Database("database", "mysql", "mariadb", "postgresql", "sqlite")
	db.Connections[0].ConfigKeys = map[string]string{"database": "database"}
	db.Connections[0].Defaults = setupkit.Values{
		"host": "localhost", "mysql.username": "root", "mariadb.username": "root", "database": "mliev_iam",
		"sqlite.path": filepath.Join(filepath.Dir(configPath), "mliev_iam.db"),
	}
	admin := presets.Admin()
	for i := range admin.Fields {
		if admin.Fields[i].Key == "admin.username" {
			admin.Fields[i].Pattern = utils.UsernamePattern
			admin.Fields[i].Help = "3–50 位字母、数字或下划线，纯数字账户保留前导零。"
			admin.Fields[i].HelpKey = ""
		}
	}
	admin.Fields = append(admin.Fields,
		setupkit.Field{Key: "admin.realName", Label: "姓名", Type: "text", Required: true, MaxLength: 50},
		setupkit.Field{Key: "admin.phone", Label: "手机号", Type: "text", MaxLength: 20})
	redis := presets.Redis("redis", nil)
	// Installation presets must not affect startup after configuration is saved.
	if !installed {
		bindSetupEnvironment(&db, &redis, &admin)
	}
	var agreements []setupkit.Agreement
	// The standalone community entrypoint leaves Edition empty.
	if edition == "" || edition == "community" {
		agreements = []setupkit.Agreement{{
			ID: "mit-license", Title: "开源版授权协议（MIT License）",
			FS: setupLicenseFS, Path: "setup_license.txt",
		}}
	}
	jwtConfigured := os.Getenv("JWT_SECRET_KEY") != "" || viper.GetString("jwt.secret") != ""
	kit, err := setupkit.New(setupkit.Options{
		Name: presentation.Name, Description: "配置数据库与 Redis，创建系统管理员。",
		BasePath: "/install", RedirectURL: "/install/enter", Logo: "/install/logo.svg",
		Store:      &installation.SetupStore{FileStore: setupkit.FileStore{ConfigPath: configPath}, InstalledAtStartup: installed},
		Steps:      []setupkit.Step{db, redis, admin},
		Agreements: agreements,
		Checkers: []setupkit.Checker{
			setupMySQLChecker{Checker: mysqlcheck.New(), id: "mysql", title: "MySQL"},
			setupMySQLChecker{Checker: mysqlcheck.New(), id: "mariadb", title: "MariaDB"},
			pgcheck.New(), sqlitecheck.New(), redischeck.New(),
		},
		AllowedOrigins: viper.GetStringSlice("install.allowed_origins"),
		Actions: []setupkit.InstallAction{
			{ID: "credentials", Title: "准备应用密钥", Run: func(ctx context.Context, in *setupkit.InstallInput) error {
				if err := ctx.Err(); err != nil {
					return err
				}
				if err := installation.EnsureCredentialKey(false); err != nil {
					return err
				}
				if !jwtConfigured {
					key := make([]byte, 48)
					if _, err := rand.Read(key); err != nil {
						return err
					}
					in.Settings["jwt.secret"] = base64.StdEncoding.EncodeToString(key)
				}
				return nil
			}},
			{ID: "migrate", Title: "迁移数据库", Run: func(ctx context.Context, in *setupkit.InstallInput) error {
				return withSetupDatabase(ctx, in.Connections["database"], func(db *gorm.DB, sqlDB *sql.DB) error {
					return migrations.RunGooseMigrationsWithGormContext(ctx, sqlDB, db, in.Connections["database"].Driver, nil)
				})
			}},
			{ID: "initialize", Title: "初始化系统与管理员", Run: func(ctx context.Context, in *setupkit.InstallInput) error {
				return withSetupDatabase(ctx, in.Connections["database"], func(db *gorm.DB, _ *sql.DB) error {
					return initializeInstallationOnce(db, dto.AdminAccountInfo{
						Username: in.Values.String("admin.username"), Password: in.Values.String("admin.password"),
						Email: in.Values.String("admin.email"), RealName: in.Values.String("admin.realName"),
						Phone: in.Values.String("admin.phone"),
					}, in.ID)
				})
			}},
		},
		AfterInstall: func(context.Context) error { reload.Restart(); return nil },
	})
	if err != nil {
		return nil, err
	}
	return &SetupInstaller{Kit: kit, ready: ready, entryPath: presentation.EntryPath, enterHTML: enterHTML.Bytes()}, nil
}

func withSetupDatabase(ctx context.Context, connection setupkit.ConnectionValues, run func(*gorm.DB, *sql.DB) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	v := connection.Values
	cfg := dbConfig.DatabaseConfig{Driver: connection.Driver, Host: v.String("host"), Port: v.Int("port"),
		Username: v.String("username"), Password: v.String("password"), DBName: v.String("database"),
		Charset: v.String("charset"), TLS: v.Bool("tls"), PreferSimpleProtocol: true}
	var sqlDB *sql.DB
	var dialector gorm.Dialector
	switch connection.Driver {
	case "mysql", "mariadb":
		driverConfig, err := cfg.MySQLDriverConfig()
		if err != nil {
			return err
		}
		driverConfig.Timeout = 5 * time.Second
		connector, err := mysqldriver.NewConnector(driverConfig)
		if err != nil {
			return err
		}
		sqlDB = sql.OpenDB(connector)
		// GORM's own version query uses a background context. Read it here so
		// MariaDB gets compatible JSON writes while installation stays cancellable.
		var serverVersion string
		if err := sqlDB.QueryRowContext(ctx, "SELECT VERSION()").Scan(&serverVersion); err != nil {
			sqlDB.Close()
			return err
		}
		dialector = mysql.New(mysql.Config{Conn: sqlDB, ServerVersion: serverVersion, SkipInitializeWithVersion: true})
	case "postgresql":
		driverConfig, err := cfg.PostgreSQLDriverConfig()
		if err != nil {
			return err
		}
		driverConfig.ConnectTimeout = 5 * time.Second
		sqlDB = stdlib.OpenDB(*driverConfig)
		dialector = postgres.New(postgres.Config{Conn: sqlDB, PreferSimpleProtocol: true})
	case "sqlite":
		dialector = sqlite.Open(v.String("path"))
	default:
		return setupkit.Failure("validation", "不支持的数据库驱动")
	}
	if sqlDB != nil {
		defer sqlDB.Close()
	}
	db, err := gorm.Open(dialector, &gorm.Config{DisableAutomaticPing: true, Logger: logger.Discard})
	if err != nil {
		return err
	}
	if sqlDB == nil {
		sqlDB, err = db.DB()
		if err != nil {
			return err
		}
		defer sqlDB.Close()
	}
	if err := sqlDB.PingContext(ctx); err != nil {
		return err
	}
	return run(db.WithContext(ctx), sqlDB)
}

func (s *SetupInstaller) Register(router httpInterfaces.RouterInterface) {
	s.Kit.Register(router)
	router.GET("/install/logo.svg", func(c httpInterfaces.RouterContextInterface) { c.Data(http.StatusOK, "image/svg+xml", setupLogo) })
	router.GET("/favicon.ico", func(c httpInterfaces.RouterContextInterface) { c.Data(http.StatusOK, "image/x-icon", setupFavicon) })
	router.GET("/install/enter", func(c httpInterfaces.RouterContextInterface) {
		c.SetHeader("Cache-Control", "no-store")
		c.SetHeader("Content-Security-Policy", "default-src 'none'; script-src 'self'; connect-src 'self'; style-src 'unsafe-inline'; frame-ancestors 'none'")
		if s.ready() {
			c.Redirect(http.StatusFound, s.entryPath)
			return
		}
		if !s.Kit.Status().Installed {
			c.Redirect(http.StatusFound, "/install/")
			return
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", s.enterHTML)
	})
	router.GET("/install/enter.js", func(c httpInterfaces.RouterContextInterface) {
		c.SetHeader("Cache-Control", "no-store")
		c.Data(http.StatusOK, "text/javascript; charset=utf-8", setupEnterJS)
	})
}

// Installed is durable; ready belongs to the current process and becomes true
// only after runtime connections, core migrations and edition startup succeed.
func (s *SetupInstaller) Middleware() httpInterfaces.HandlerFunc {
	return func(c httpInterfaces.RouterContextInterface) {
		p := path.Clean("/" + strings.TrimPrefix(c.Path(), "/"))
		if p == "/install" || strings.HasPrefix(p, "/install/") || p == "/health" || strings.HasPrefix(p, "/health/") || p == "/api/install/check" || p == "/favicon.ico" {
			c.Next()
			return
		}
		if s.ready() {
			c.Next()
			return
		}
		if c.Method() == http.MethodGet && strings.Contains(c.GetHeader("Accept"), "text/html") {
			target := "/install/"
			if s.Kit.Status().Installed {
				target = "/install/enter"
			}
			c.Redirect(http.StatusFound, target)
			c.Abort()
			return
		}
		c.AbortWithStatusJSON(http.StatusServiceUnavailable, &setupkit.PublicError{Code: "unavailable", Message: "请完成安装并等待服务启动"})
	}
}

func (*SetupInstaller) Run() error { return nil }
func (s *SetupInstaller) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return s.Kit.Close(ctx)
}
