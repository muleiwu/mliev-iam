package service

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type fakeThemeObjects struct {
	data        []byte
	reads, puts int
	failAt      int
	deleted     []string
}

func (f *fakeThemeObjects) MaxFileBytes() int64 { return 5 << 20 }
func (f *fakeThemeObjects) Read(context.Context, string) ([]byte, string, error) {
	f.reads++
	if f.data == nil {
		return nil, "", errors.New("missing object")
	}
	return f.data, "image/png", nil
}
func (f *fakeThemeObjects) Put(_ context.Context, _ []byte, _ string) (string, error) {
	f.puts++
	key := uuid.NewString()
	if f.puts == f.failAt {
		return key, errors.New("storage failed")
	}
	return key, nil
}
func (f *fakeThemeObjects) Delete(_ context.Context, key string) error {
	f.deleted = append(f.deleted, key)
	return nil
}
func themeTestService(t *testing.T) (*AppearanceThemeService, *fakeThemeObjects) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&model.AppearanceTheme{}, &model.Setting{}, &model.Attachment{}))
	store := &fakeThemeObjects{data: themePNG(t)}
	service := NewAppearanceThemeService(db, fstest.MapFS{"preview/index.html": &fstest.MapFile{Data: []byte("trusted preview")}, "preview/canvas.html": &fstest.MapFile{Data: []byte("trusted canvas")}, "preview.mjs": &fstest.MapFile{Data: []byte("trusted launcher")}, "README.md": &fstest.MapFile{Data: []byte("See external-resources.json")}})
	service.objects = func(context.Context) (themeObjectStore, error) { return store, nil }
	return service, store
}
func themePNG(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	i := image.NewRGBA(image.Rect(0, 0, 2, 2))
	i.Set(0, 0, color.RGBA{R: 80, G: 90, B: 100, A: 255})
	require.NoError(t, png.Encode(&buf, i))
	return buf.Bytes()
}
func fixtureThemeZip(t *testing.T, mutate func(map[string][]byte)) []byte {
	t.Helper()
	c := DefaultAppearanceSettings().Custom
	c.Preset = "paper"
	encoded, err := themeAppearanceJSON(c)
	require.NoError(t, err)
	manifest := dto.ThemeManifest{ThemeMetadata: dto.ThemeMetadata{Name: "Example", Version: "1.0.0"}, FormatVersion: 1, Appearance: encoded, Assets: []dto.ThemeAsset{}}
	data, _ := json.Marshal(manifest)
	files := map[string][]byte{"theme.json": data, "theme.css": []byte(".auth-entry-title { letter-spacing:.02em }")}
	if mutate != nil {
		mutate(files)
	}
	data, err = writeThemeZip(files)
	require.NoError(t, err)
	return data
}
func mutateManifest(t *testing.T, files map[string][]byte, change func(*dto.ThemeManifest)) {
	t.Helper()
	var m dto.ThemeManifest
	require.NoError(t, json.Unmarshal(files["theme.json"], &m))
	change(&m)
	files["theme.json"], _ = json.Marshal(m)
}

func TestThemePackageAllPresetsRoundTrip(t *testing.T) {
	for _, preset := range []string{"default", "minimal", "brand", "midnight", "paper", "grove", "graphite"} {
		t.Run(preset, func(t *testing.T) {
			svc, store := themeTestService(t)
			c := DefaultAppearanceSettings()
			c.Mode = "custom"
			c.Custom.Preset = preset
			c.Custom.DesktopAlignment = "right"
			c.Custom.BrandName = "未保存品牌"
			c.Custom.CustomCSS = ".auth-entry-title { letter-spacing:.03em }"
			raw, err := svc.Export(context.Background(), dto.ThemeExportRequest{ThemeMetadata: dto.ThemeMetadata{Name: preset, Version: "1.0.0"}, Settings: c}, "https://iam.example")
			require.NoError(t, err)
			theme, err := svc.Import(context.Background(), raw, 1)
			require.NoError(t, err)
			require.Equal(t, preset, theme.Appearance.Preset)
			require.Equal(t, c.Custom.CustomCSS, theme.Appearance.CustomCSS)
			require.Equal(t, "right", theme.Appearance.DesktopAlignment)
			require.Equal(t, "未保存品牌", theme.Appearance.BrandName)
			require.NotEmpty(t, theme.Appearance.SourceThemeID)
			require.Zero(t, store.puts)
			active, err := NewSettingService(svc.db).GetPublicAppearance()
			require.NoError(t, err)
			require.Equal(t, "default", active.Mode)
			_, err = svc.Import(context.Background(), raw, 1)
			require.ErrorIs(t, err, ErrThemeConflict)
			require.NoError(t, NewSettingService(svc.db).UpdateAppearanceSettings(dto.AppearanceSettings{Mode: "custom", Custom: *theme.Appearance}))
			require.ErrorIs(t, svc.Delete(theme.ID), ErrThemeInUse)
			// Generic setting writes share the association and active-delete guard.
			exported, _ := json.Marshal(dto.AppearanceSettings{Mode: "custom", Custom: *theme.Appearance})
			require.NoError(t, NewSettingService(svc.db).SetSimple(AppearanceSettingsConfigKey, string(exported)))
			require.ErrorIs(t, svc.Delete(theme.ID), ErrThemeInUse)
			list, err := svc.List(1, 5)
			require.NoError(t, err)
			require.EqualValues(t, 1, list.Total)
			require.True(t, list.Items[0].InUse)
			require.Nil(t, list.Items[0].Appearance)
			config := dto.AppearanceSettings{Mode: "default", Custom: *theme.Appearance}
			require.NoError(t, NewSettingService(svc.db).UpdateAppearanceSettings(config))
			require.NoError(t, svc.Delete(theme.ID))
			stored, err := NewSettingService(svc.db).GetAppearanceSettings()
			require.NoError(t, err)
			require.Empty(t, stored.Custom.SourceThemeID)
			require.Equal(t, c.Custom.CustomCSS, stored.Custom.CustomCSS)
		})
	}
}
func TestDefaultThemeExportExcludesInactiveDraft(t *testing.T) {
	svc, _ := themeTestService(t)
	config := DefaultAppearanceSettings()
	config.Custom.BrandName = "inactive-secret-brand"
	config.Custom.CustomCSS = "body{display:none}"
	config.Custom.Logo = "/attachments/invalid"
	raw, err := svc.Export(context.Background(), dto.ThemeExportRequest{ThemeMetadata: dto.ThemeMetadata{Name: "Default", Version: "1.0.0"}, Settings: config}, "https://iam.example")
	require.NoError(t, err)
	files, err := readThemeArchive(raw)
	require.NoError(t, err)
	require.Empty(t, files["theme.css"])
	require.NotContains(t, string(files["theme.json"]), "inactive-secret-brand")
	require.NotContains(t, string(files["theme.json"]), "sourceThemeId")
	require.NotContains(t, string(files["theme.json"]), "customCss")
}
func TestThemeImagesAndEscapedCSSRoundTrip(t *testing.T) {
	svc, objects := themeTestService(t)
	id := uuid.NewString()
	c := DefaultAppearanceSettings()
	c.Mode = "custom"
	c.Custom.Logo = "/attachments/" + id
	c.Custom.BackgroundMode = "image"
	c.Custom.BackgroundImage = "https://iam.example/attachments/" + id
	c.Custom.CustomCSS = `/* url('/attachments/not-an-image') */ .a { background: URL("\2f attachments/` + id + `"); content:"/attachments/plain"; --image:url(/attachments/` + id + `); mask: image-set("/attachments/` + id + `" 1x); } .external { background:url(https://cdn.example/a.png) }`
	raw, err := svc.Export(context.Background(), dto.ThemeExportRequest{ThemeMetadata: dto.ThemeMetadata{Name: "Images", Version: "1.0.0"}, Settings: c}, "https://iam.example")
	require.NoError(t, err)
	require.Equal(t, 1, objects.reads)
	files, err := readThemeArchive(raw)
	require.NoError(t, err)
	require.Contains(t, string(files["theme.css"]), "/attachments/plain")
	require.Contains(t, string(files["theme.css"]), "/attachments/not-an-image")
	require.Contains(t, string(files["external-resources.json"]), "https://cdn.example/a.png")
	theme, err := svc.Import(context.Background(), raw, 7)
	require.NoError(t, err)
	require.Equal(t, 1, objects.puts)
	require.Equal(t, theme.Appearance.Logo, theme.Appearance.BackgroundImage)
	require.Contains(t, theme.Appearance.CustomCSS, theme.Appearance.Logo)
	require.NotContains(t, theme.Appearance.CustomCSS, "assets/")
	require.NoError(t, svc.Delete(theme.ID))
	var count int64
	require.NoError(t, svc.db.Model(&model.Attachment{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
}
func TestThemeImportRollback(t *testing.T) {
	svc, objects := themeTestService(t)
	objects.failAt = 2
	raw := fixtureThemeZip(t, func(files map[string][]byte) {
		files["assets/a.png"] = themePNG(t)
		files["assets/b.png"] = themePNG(t)
		mutateManifest(t, files, func(m *dto.ThemeManifest) {
			m.Assets = []dto.ThemeAsset{{Path: "assets/a.png", MimeType: "image/png"}, {Path: "assets/b.png", MimeType: "image/png"}}
		})
	})
	_, err := svc.Import(context.Background(), raw, 1)
	require.Error(t, err)
	require.Len(t, objects.deleted, 2)
	for _, table := range []string{"appearance_themes", "attachments", "settings"} {
		var count int64
		require.NoError(t, svc.db.Table(table).Count(&count).Error)
		require.Zero(t, count)
	}
}
func TestThemePackageRejectsInvalidContents(t *testing.T) {
	cases := map[string]func(map[string][]byte){
		"version":             func(f map[string][]byte) { mutateManifest(t, f, func(m *dto.ThemeManifest) { m.FormatVersion = 2 }) },
		"missing css":         func(f map[string][]byte) { delete(f, "theme.css") },
		"css limit":           func(f map[string][]byte) { f["theme.css"] = bytes.Repeat([]byte("a"), 32769) },
		"broken css":          func(f map[string][]byte) { f["theme.css"] = []byte(`a { background:url("unterminated) }`) },
		"missing asset":       func(f map[string][]byte) { f["theme.css"] = []byte(`a {background:url(assets/missing.png)}`) },
		"traversal reference": func(f map[string][]byte) { f["theme.css"] = []byte(`a {background:url(assets/%2e%2e/secret.png)}`) },
		"invalid image": func(f map[string][]byte) {
			f["assets/a.png"] = []byte("not a png")
			mutateManifest(t, f, func(m *dto.ThemeManifest) { m.Assets = []dto.ThemeAsset{{Path: "assets/a.png", MimeType: "image/png"}} })
		},
		"fake type": func(f map[string][]byte) {
			f["assets/a.png"] = themePNG(t)
			mutateManifest(t, f, func(m *dto.ThemeManifest) {
				m.Assets = []dto.ThemeAsset{{Path: "assets/a.png", MimeType: "image/jpeg"}}
			})
		},
		"nested theme": func(f map[string][]byte) {
			mutateManifest(t, f, func(m *dto.ThemeManifest) { m.Appearance = json.RawMessage(`{"sourceThemeId":"another"}`) })
		},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := parseThemePackage(fixtureThemeZip(t, change))
			require.ErrorIs(t, err, ErrThemePackage)
		})
	}
}
func TestThemeArchiveEntryLimitsAndWrapper(t *testing.T) {
	for _, name := range []string{"../theme.json", "/theme.json", "C:/theme.json", "a\\theme.json"} {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			z := zip.NewWriter(&buf)
			w, _ := z.Create(name)
			w.Write([]byte("{}"))
			z.Close()
			_, err := readThemeArchive(buf.Bytes())
			require.ErrorIs(t, err, ErrThemePackage)
		})
	}
	var duplicate bytes.Buffer
	z := zip.NewWriter(&duplicate)
	z.Create("theme.json")
	z.Create("THEME.JSON")
	z.Close()
	_, err := readThemeArchive(duplicate.Bytes())
	require.ErrorIs(t, err, ErrThemePackage)
	var links bytes.Buffer
	z = zip.NewWriter(&links)
	h := &zip.FileHeader{Name: "theme.json"}
	h.SetMode(fs.ModeSymlink | 0777)
	z.CreateHeader(h)
	z.Close()
	_, err = readThemeArchive(links.Bytes())
	require.ErrorIs(t, err, ErrThemePackage)
	var many bytes.Buffer
	z = zip.NewWriter(&many)
	for i := 0; i < 129; i++ {
		z.Create(uuid.NewString())
	}
	z.Close()
	_, err = readThemeArchive(many.Bytes())
	require.ErrorIs(t, err, ErrThemePackage)
	_, err = readThemeArchive(make([]byte, ThemePackageMaxZip+1))
	require.ErrorIs(t, err, ErrThemePackage)
	files, err := readThemeArchive(fixtureThemeZip(t, nil))
	require.NoError(t, err)
	wrapped := map[string][]byte{}
	for name, data := range files {
		wrapped["MyTheme/"+name] = data
	}
	raw, err := writeThemeZip(wrapped)
	require.NoError(t, err)
	_, err = parseThemePackage(raw)
	require.NoError(t, err)
}
func TestCSSURLSyntaxTraversal(t *testing.T) {
	css := `.a{ --x:u\72l("assets/a\20 b.png");background:image-set('assets/c.png' 1x,url(assets/d.png) 2x); content:"url(assets/not.png)";} /*url(assets/comment.png)*/ @import "https://example.com/base.css";`
	var seen []string
	rewritten, err := rewriteThemeCSS(css, func(raw string) (string, error) { seen = append(seen, raw); return "/mapped/" + pathBase(raw), nil })
	require.NoError(t, err)
	require.Equal(t, []string{"assets/a b.png", "assets/c.png", "assets/d.png", "https://example.com/base.css"}, seen)
	require.Contains(t, rewritten, `content:"url(assets/not.png)"`)
	require.Contains(t, rewritten, "/*url(assets/comment.png)*/")
}
func pathBase(value string) string { return value[strings.LastIndex(value, "/")+1:] }

// Explicit browser smoke fixture: run after pnpm build with a destination in /tmp.
func TestExportBuiltThemeKit(t *testing.T) {
	dest := os.Getenv("IAM_THEME_KIT_EXPORT_PATH")
	if dest == "" {
		t.Skip("set IAM_THEME_KIT_EXPORT_PATH for the standalone browser fixture")
	}
	svc, _ := themeTestService(t)
	svc.kit = os.DirFS("../../static/auth/theme-kit")
	config := DefaultAppearanceSettings()
	config.Mode = "custom"
	config.Custom.Preset = "paper"
	config.Custom.DesktopAlignment = "right"
	config.Custom.BrandName = "主题开发示例"
	config.Custom.CustomCSS = ".auth-entry-title{letter-spacing:1px}"
	data, err := svc.Export(context.Background(), dto.ThemeExportRequest{ThemeMetadata: dto.ThemeMetadata{Name: "本地开发示例", Version: "1.0.0"}, Settings: config}, "https://iam.example")
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(dest), 0755))
	require.NoError(t, os.WriteFile(dest, data, 0600))
}

func TestThemeArchiveRejectsExpandedBomb(t *testing.T) {
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	w, err := z.Create("preview/oversize.js")
	require.NoError(t, err)
	_, err = io.CopyN(w, strings.NewReader(strings.Repeat("a", ThemePackageMaxExpanded+1)), ThemePackageMaxExpanded+1)
	require.NoError(t, err)
	require.NoError(t, z.Close())
	require.Less(t, buf.Len(), ThemePackageMaxZip)
	_, err = readThemeArchive(buf.Bytes())
	require.ErrorIs(t, err, ErrThemePackage)
}
func TestImportedPreviewCodeIsNeverStoredOrReexported(t *testing.T) {
	svc, _ := themeTestService(t)
	raw := fixtureThemeZip(t, func(files map[string][]byte) {
		files["preview.mjs"] = []byte("throw Error('untrusted')")
		files["preview/index.html"] = []byte("<script>untrusted()</script>")
	})
	theme, err := svc.Import(context.Background(), raw, 1)
	require.NoError(t, err)
	edited := *theme.Appearance
	edited.CustomCSS = "body { color: blue }"
	require.NoError(t, NewSettingService(svc.db).UpdateAppearanceSettings(dto.AppearanceSettings{Mode: "custom", Custom: edited}))
	original, err := svc.Get(theme.ID)
	require.NoError(t, err)
	require.NotEqual(t, edited.CustomCSS, original.Appearance.CustomCSS)
	exported, err := svc.Export(context.Background(), dto.ThemeExportRequest{ThemeMetadata: dto.ThemeMetadata{Name: "Safe", Version: "1.0.0"}, Settings: dto.AppearanceSettings{Mode: "custom", Custom: edited}}, "https://iam.example")
	require.NoError(t, err)
	files, err := readThemeArchive(exported)
	require.NoError(t, err)
	require.Equal(t, "trusted launcher", string(files["preview.mjs"]))
}

func TestLiveAppearanceDoesNotDependOnThemeLibraryAvailability(t *testing.T) {
	svc, _ := themeTestService(t)
	theme, err := svc.Import(context.Background(), fixtureThemeZip(t, nil), 1)
	require.NoError(t, err)
	settings := NewSettingService(svc.db)
	require.NoError(t, settings.UpdateAppearanceSettings(dto.AppearanceSettings{Mode: "custom", Custom: *theme.Appearance}))
	require.NoError(t, svc.db.Migrator().DropTable(&model.AppearanceTheme{}))
	active, err := settings.GetPublicAppearance()
	require.NoError(t, err)
	require.Equal(t, "custom", active.Mode)
	require.Equal(t, theme.Appearance.CustomCSS, active.Appearance.CustomCSS)
	require.Equal(t, theme.Appearance.Preset, active.Appearance.Preset)
}
