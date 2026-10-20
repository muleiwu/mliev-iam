package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"unicode"

	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"gorm.io/gorm"
)

const AppearanceSettingsConfigKey = "appearance_settings_config"

var appearanceColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func DefaultAppearanceSettings() dto.AppearanceSettings {
	return dto.AppearanceSettings{Mode: "default", Custom: dto.AppearanceConfig{Preset: "default", DesktopAlignment: "center", BackgroundMode: "default"}}
}

func validAppearancePreset(preset string) bool {
	return preset == "default" || preset == "minimal" || preset == "brand" || preset == "midnight" || preset == "paper" || preset == "grove" || preset == "graphite"
}

// ResolveAppearanceConfig derives client-facing defaults without persisting them as overrides.
// Keep these defaults in sync with frontend-oidc/src/themes/presets.ts.
func ResolveAppearanceConfig(config dto.AppearanceConfig) dto.AppearanceConfig {
	var color, theme string
	cardRadius, controlRadius := 16, 8
	switch config.Preset {
	case "minimal":
		color, theme, cardRadius, controlRadius = "#334155", "light", 8, 6
	case "brand":
		color, theme = "#1d4ed8", "auto"
	case "midnight":
		color, theme = "#0f766e", "dark"
	case "paper":
		color, theme, cardRadius, controlRadius = "#9a4c20", "light", 4, 4
	case "grove":
		color, theme, cardRadius, controlRadius = "#3f6212", "auto", 24, 12
	case "graphite":
		color, theme, cardRadius, controlRadius = "#3f4c5a", "auto", 0, 0
	default:
		return config
	}
	if config.PrimaryColor == "" {
		config.PrimaryColor = color
	}
	if config.DarkMode == "" {
		config.DarkMode = theme
	}
	if config.CardRadius == nil {
		config.CardRadius = &cardRadius
	}
	if config.ControlRadius == nil {
		config.ControlRadius = &controlRadius
	}
	return config
}

func ValidateAppearanceSettings(config *dto.AppearanceSettings) error {
	if config.Mode != "default" && config.Mode != "custom" {
		return errors.New("风格必须为默认或自定义")
	}
	c := &config.Custom
	if c.Preset == "" {
		c.Preset = "default"
	}
	if !validAppearancePreset(c.Preset) {
		return errors.New("风格预设无效")
	}
	if c.DesktopAlignment == "" {
		c.DesktopAlignment = "center"
	}
	if c.DesktopAlignment != "center" && c.DesktopAlignment != "right" {
		return errors.New("PC 端卡片位置必须为居中或居右")
	}
	c.BrandName = strings.TrimSpace(c.BrandName)
	if len([]rune(c.BrandName)) > 100 {
		return errors.New("品牌名称不能超过 100 个字符")
	}
	for _, color := range []string{c.PrimaryColor, c.BackgroundColor} {
		if color != "" && !appearanceColor.MatchString(color) {
			return errors.New("颜色必须为 #RRGGBB 格式")
		}
	}
	if c.DarkMode != "" && c.DarkMode != "auto" && c.DarkMode != "light" && c.DarkMode != "dark" {
		return errors.New("明暗模式无效")
	}
	if c.BackgroundMode == "" {
		c.BackgroundMode = "default"
	}
	if c.BackgroundMode != "default" && c.BackgroundMode != "color" && c.BackgroundMode != "image" {
		return errors.New("背景模式无效")
	}
	if c.BackgroundMode == "color" && c.BackgroundColor == "" {
		return errors.New("请选择背景颜色")
	}
	if c.BackgroundMode == "image" && c.BackgroundImage == "" {
		return errors.New("请配置背景图片")
	}
	for _, raw := range []string{c.Logo, c.BackgroundImage} {
		if raw == "" {
			continue
		}
		if len(raw) > 2048 || strings.ContainsAny(raw, `\`) || strings.IndexFunc(raw, unicode.IsSpace) >= 0 || strings.IndexFunc(raw, unicode.IsControl) >= 0 {
			return errors.New("图片地址无效")
		}
		u, err := url.Parse(raw)
		if err != nil || u.User != nil || !((strings.HasPrefix(raw, "/") && !strings.HasPrefix(raw, "//") && u.Host == "") || ((u.Scheme == "http" || u.Scheme == "https") && u.Host != "")) {
			return errors.New("图片地址必须为站内路径或 HTTP(S) 地址")
		}
	}
	if c.CardRadius != nil && (*c.CardRadius < 0 || *c.CardRadius > 48) {
		return errors.New("卡片圆角须在 0–48 px 之间")
	}
	if c.ControlRadius != nil && (*c.ControlRadius < 0 || *c.ControlRadius > 32) {
		return errors.New("控件圆角须在 0–32 px 之间")
	}
	if len(c.CustomCSS) > 32*1024 {
		return errors.New("自定义 CSS 不能超过 32 KiB")
	}
	data, err := json.Marshal(config)
	if err != nil || len(data) > 60*1024 {
		return errors.New("界面配置不能超过 60 KiB")
	}
	return nil
}

// Read directly, like the other structured settings, so every instance sees saves immediately.
func (s *SettingService) GetAppearanceSettings() (dto.AppearanceSettings, error) {
	defaults := DefaultAppearanceSettings()
	setting, err := s.settingDao.GetByKey(AppearanceSettingsConfigKey)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return defaults, nil
	}
	if err != nil {
		return defaults, err
	}
	var config dto.AppearanceSettings
	if json.Unmarshal([]byte(setting.Value), &config) != nil {
		return defaults, nil
	}
	// Future/unknown choices must not erase the rest of a saved customization.
	if !validAppearancePreset(config.Custom.Preset) {
		config.Custom.Preset = "default"
	}
	if config.Custom.DesktopAlignment != "center" && config.Custom.DesktopAlignment != "right" {
		config.Custom.DesktopAlignment = "center"
	}
	if ValidateAppearanceSettings(&config) != nil {
		return defaults, nil
	}
	return config, nil
}

func (s *SettingService) UpdateAppearanceSettings(config dto.AppearanceSettings) error {
	if err := ValidateAppearanceSettings(&config); err != nil {
		return err
	}
	data, _ := json.Marshal(config)
	return s.settingDao.CreateOrUpdate(&model.Setting{
		Key: AppearanceSettingsConfigKey, Value: string(data), Category: "appearance", Remark: "认证门户界面设置",
	})
}

func (s *SettingService) GetPublicAppearance() (dto.PublicAppearanceResponse, error) {
	config, err := s.GetAppearanceSettings()
	active := DefaultAppearanceSettings().Custom
	if config.Mode == "custom" {
		active = config.Custom
	}
	return dto.PublicAppearanceResponse{Mode: config.Mode, Appearance: active}, err
}

// Generic settings endpoints must enforce the same contract as the dedicated editor.
func validateAppearanceSettingValue(key, value string) error {
	if key != AppearanceSettingsConfigKey {
		return nil
	}
	if len(value) > 60*1024 {
		return errors.New("界面配置不能超过 60 KiB")
	}
	var config dto.AppearanceSettings
	if err := json.Unmarshal([]byte(value), &config); err != nil {
		return fmt.Errorf("界面配置 JSON 无效: %w", err)
	}
	return ValidateAppearanceSettings(&config)
}
