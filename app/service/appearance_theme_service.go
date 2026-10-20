package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"path"
	"sort"
	"strings"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrThemeNotFound = errors.New("主题不存在")

type AppearanceThemeService struct {
	db      *gorm.DB
	kit     fs.FS
	objects func(context.Context) (themeObjectStore, error)
}

func NewAppearanceThemeService(db *gorm.DB, kit fs.FS) *AppearanceThemeService {
	return &AppearanceThemeService{db: db, kit: kit, objects: func(ctx context.Context) (themeObjectStore, error) { return newThemeObjectStore(ctx, db) }}
}
func themeIdentity(name, version string) string {
	h := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(name)) + "\x00" + version))
	return hex.EncodeToString(h[:])
}
func themeResponse(row model.AppearanceTheme, detail bool, active string) (dto.AppearanceThemeResponse, error) {
	result := dto.AppearanceThemeResponse{ThemeMetadata: dto.ThemeMetadata{Name: row.Name, Version: row.Version, Author: row.Author, Description: row.Description}, ID: row.UUID, FormatVersion: row.FormatVersion, CreatedAt: row.CreatedAt, InUse: row.UUID == active}
	if detail {
		var c dto.AppearanceConfig
		if err := json.Unmarshal([]byte(row.AppearanceJSON), &c); err != nil {
			return result, err
		}
		c.SourceThemeID = row.UUID
		result.Appearance = &c
	}
	return result, nil
}
func (s *AppearanceThemeService) activeID() string {
	config, err := NewSettingService(s.db).GetAppearanceSettings()
	if err == nil && config.Mode == "custom" {
		return config.Custom.SourceThemeID
	}
	return ""
}
func (s *AppearanceThemeService) List(pageNumber, pageSize int) (dto.AppearanceThemeList, error) {
	if pageNumber < 1 {
		pageNumber = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	result := dto.AppearanceThemeList{Items: []dto.AppearanceThemeResponse{}, Page: pageNumber, PageSize: pageSize}
	if err := s.db.Model(&model.AppearanceTheme{}).Count(&result.Total).Error; err != nil {
		return result, err
	}
	var rows []model.AppearanceTheme
	err := s.db.Select("id", "uuid", "name", "version", "author", "description", "format_version", "created_at").Order("id DESC").Offset((pageNumber - 1) * pageSize).Limit(pageSize).Find(&rows).Error
	if err != nil {
		return result, err
	}
	active := s.activeID()
	for _, row := range rows {
		item, _ := themeResponse(row, false, active)
		result.Items = append(result.Items, item)
	}
	return result, nil
}
func (s *AppearanceThemeService) Get(id string) (dto.AppearanceThemeResponse, error) {
	var row model.AppearanceTheme
	if err := s.db.Where("uuid = ?", id).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = ErrThemeNotFound
		}
		return dto.AppearanceThemeResponse{}, err
	}
	return themeResponse(row, true, s.activeID())
}
func (s *AppearanceThemeService) Delete(id string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var row model.AppearanceTheme
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("uuid = ?", id).First(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrThemeNotFound
			}
			return err
		}
		config, err := NewSettingService(tx).GetAppearanceSettings()
		if err != nil {
			return err
		}
		if config.Mode == "custom" && config.Custom.SourceThemeID == id {
			return ErrThemeInUse
		}
		// Snapshot URLs may be shared by another customization; never delete its attachments here.
		return tx.Delete(&row).Error
	})
}

func (s *AppearanceThemeService) Import(ctx context.Context, data []byte, userID uint) (dto.AppearanceThemeResponse, error) {
	var result dto.AppearanceThemeResponse
	parsed, err := parseThemePackage(data)
	if err != nil {
		return result, err
	}
	identity := themeIdentity(parsed.Manifest.Name, parsed.Manifest.Version)
	var count int64
	if err = s.db.Model(&model.AppearanceTheme{}).Where("identity_key = ?", identity).Count(&count).Error; err != nil {
		return result, err
	}
	if count > 0 {
		return result, ErrThemeConflict
	}
	var store themeObjectStore
	if len(parsed.Assets) > 0 {
		store, err = s.objects(ctx)
		if err != nil {
			return result, err
		}
		for name, data := range parsed.Assets {
			if int64(len(data)) > store.MaxFileBytes() {
				return result, packageError("图片 %s 超过存储设置的大小限制", name)
			}
		}
	}
	var uploaded []string
	var row model.AppearanceTheme
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		urls := map[string]string{}
		for _, asset := range parsed.Manifest.Assets {
			bytes := parsed.Assets[asset.Path]
			key, err := store.Put(ctx, bytes, asset.MimeType)
			if key != "" {
				uploaded = append(uploaded, key)
			}
			if err != nil {
				return err
			}
			attachment := model.Attachment{UUID: uuid.NewString(), FileName: path.Base(asset.Path), FileSize: int64(len(bytes)), MimeType: asset.MimeType, StorageType: model.AttachmentStorageTypeS3, StorageKey: key, BizType: "brand_image", UploaderUserID: userID}
			if err = tx.Create(&attachment).Error; err != nil {
				return err
			}
			urls[asset.Path] = "/attachments/" + attachment.UUID
		}
		config, err := parsed.rewrite(func(name string) (string, error) { return urls[name], nil })
		if err != nil {
			return err
		}
		settings := dto.AppearanceSettings{Mode: "custom", Custom: config}
		if err = ValidateAppearanceSettings(&settings); err != nil {
			return packageError("%v", err)
		}
		encoded, _ := json.Marshal(settings.Custom)
		row = model.AppearanceTheme{UUID: uuid.NewString(), IdentityKey: identity, Name: parsed.Manifest.Name, Version: parsed.Manifest.Version, Author: parsed.Manifest.Author, Description: parsed.Manifest.Description, FormatVersion: 1, AppearanceJSON: string(encoded)}
		return tx.Create(&row).Error
	})
	if err != nil {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		for _, key := range uploaded {
			if cleanupErr := store.Delete(cleanupCtx, key); cleanupErr != nil {
				err = errors.Join(err, fmt.Errorf("清理本次上传失败: %w", cleanupErr))
			}
		}
		var duplicates int64
		if s.db.Model(&model.AppearanceTheme{}).Where("identity_key = ?", identity).Count(&duplicates).Error == nil && duplicates > 0 {
			return result, errors.Join(ErrThemeConflict, err)
		}
		return result, err
	}
	return themeResponse(row, true, "")
}

func (s *AppearanceThemeService) Export(ctx context.Context, request dto.ThemeExportRequest, origin string) ([]byte, error) {
	if err := validateThemeMetadata(&request.ThemeMetadata); err != nil {
		return nil, err
	}
	config := request.Settings.Custom
	if request.Settings.Mode == "default" {
		config = DefaultAppearanceSettings().Custom
	} else if request.Settings.Mode != "custom" {
		return nil, packageError("风格模式无效")
	}
	config.SourceThemeID = ""
	settings := dto.AppearanceSettings{Mode: "custom", Custom: config}
	if err := ValidateAppearanceSettings(&settings); err != nil {
		return nil, packageError("%v", err)
	}
	config = settings.Custom
	if s.kit == nil {
		return nil, errors.New("开发预览资源不可用，请先构建认证前端")
	}
	files := map[string][]byte{}
	var exportBytes int64
	err := fs.WalkDir(s.kit, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		reader, err := s.kit.Open(name)
		if err != nil {
			return err
		}
		data, err := io.ReadAll(io.LimitReader(reader, ThemePackageMaxExpanded-exportBytes+1))
		reader.Close()
		if err != nil {
			return err
		}
		exportBytes += int64(len(data))
		if exportBytes > ThemePackageMaxExpanded || len(files) >= ThemePackageMaxFiles-3 {
			return packageError("预览资源超过导出限制")
		}
		files[name] = data
		return nil
	})
	if err != nil || len(files["preview/index.html"]) == 0 || len(files["preview/canvas.html"]) == 0 || len(files["preview.mjs"]) == 0 {
		return nil, errors.New("开发预览资源不可用，请先构建认证前端")
	}
	base, err := url.Parse(origin)
	if err != nil || base.Host == "" {
		return nil, packageError("站点地址无效")
	}
	var store themeObjectStore
	assetPaths := map[string]string{}
	external := map[string]bool{}
	assets := []dto.ThemeAsset{}
	visit := func(raw string) (string, error) {
		if raw == "" || strings.HasPrefix(raw, "#") || strings.HasPrefix(raw, "data:") {
			return raw, nil
		}
		u, err := url.Parse(raw)
		if err != nil {
			return "", packageError("资源地址无效")
		}
		resolved := base.ResolveReference(u)
		if resolved.Scheme != "http" && resolved.Scheme != "https" {
			return "", packageError("不支持的资源协议")
		}
		if strings.EqualFold(resolved.Host, base.Host) && resolved.Scheme == base.Scheme && strings.HasPrefix(resolved.Path, "/attachments/") {
			id := strings.TrimPrefix(resolved.Path, "/attachments/")
			parsedID, err := uuid.Parse(id)
			if err != nil {
				return "", packageError("站内附件 UUID 无效")
			}
			id = parsedID.String()
			assetPath, ok := assetPaths[id]
			if !ok {
				if store == nil {
					store, err = s.objects(ctx)
					if err != nil {
						return "", err
					}
				}
				data, mime, err := store.Read(ctx, id)
				if err != nil {
					return "", fmt.Errorf("无法打包站内图片 %s: %w", id, err)
				}
				exportBytes += int64(len(data))
				if exportBytes > ThemePackageMaxExpanded || len(files) >= ThemePackageMaxFiles-3 {
					return "", packageError("导出资源超过限制")
				}
				assetPath = "assets/" + id + extensionFromImageMimeType(mime)
				files[assetPath] = data
				assetPaths[id] = assetPath
				assets = append(assets, dto.ThemeAsset{Path: assetPath, MimeType: mime})
			}
			if u.Fragment != "" {
				assetPath += "#" + u.Fragment
			}
			return assetPath, nil
		}
		absolute := resolved.String()
		external[absolute] = true
		return absolute, nil
	}
	if config.Logo != "" {
		config.Logo, err = visit(config.Logo)
		if err != nil {
			return nil, err
		}
	}
	if config.BackgroundImage != "" {
		config.BackgroundImage, err = visit(config.BackgroundImage)
		if err != nil {
			return nil, err
		}
	}
	css, err := rewriteThemeCSS(config.CustomCSS, visit)
	if err != nil {
		return nil, err
	}
	if len(css) > 32*1024 {
		return nil, packageError("资源路径展开后 CSS 超过 32 KiB")
	}
	// Validate the portable form too: absolute external URLs and rewritten CSS
	// can be longer than the editor's original references.
	portable := config
	portable.CustomCSS = css
	candidate := parsedThemePackage{Config: portable, Assets: files}
	check, err := candidate.rewrite(func(string) (string, error) { return "/attachments/00000000-0000-0000-0000-000000000000", nil })
	if err != nil {
		return nil, err
	}
	check.SourceThemeID = "00000000-0000-0000-0000-000000000000"
	verified := dto.AppearanceSettings{Mode: "custom", Custom: check}
	if err = ValidateAppearanceSettings(&verified); err != nil {
		return nil, packageError("导出后配置无效: %v", err)
	}
	appearance, err := themeAppearanceJSON(config)
	if err != nil {
		return nil, err
	}
	manifest := dto.ThemeManifest{ThemeMetadata: request.ThemeMetadata, FormatVersion: 1, Appearance: appearance, Assets: assets}
	files["theme.json"], err = json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, err
	}
	files["theme.css"] = []byte(css)
	refs := make([]string, 0, len(external))
	for ref := range external {
		refs = append(refs, ref)
	}
	sort.Strings(refs)
	resources, _ := json.MarshalIndent(refs, "", "  ")
	files["external-resources.json"] = resources
	// The starter README explains the external resources manifest; it never contains credentials.
	return writeThemeZip(files)
}
