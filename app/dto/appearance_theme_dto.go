package dto

import (
	"encoding/json"
	"time"
)

type ThemeMetadata struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Author      string `json:"author,omitempty"`
	Description string `json:"description,omitempty"`
}

type ThemeExportRequest struct {
	ThemeMetadata
	Settings AppearanceSettings `json:"settings"`
}

type ThemeAsset struct {
	Path     string `json:"path"`
	MimeType string `json:"mimeType"`
}

// Appearance intentionally excludes customCss/sourceThemeId; theme.css is authoritative.
type ThemeManifest struct {
	ThemeMetadata
	FormatVersion int             `json:"formatVersion"`
	Appearance    json.RawMessage `json:"appearance"`
	Assets        []ThemeAsset    `json:"assets"`
}

type AppearanceThemeResponse struct {
	ThemeMetadata
	ID            string            `json:"id"`
	FormatVersion int               `json:"formatVersion"`
	CreatedAt     time.Time         `json:"createdAt"`
	InUse         bool              `json:"inUse"`
	Appearance    *AppearanceConfig `json:"appearance,omitempty"`
}

type AppearanceThemeList struct {
	Items    []AppearanceThemeResponse `json:"items"`
	Total    int64                     `json:"total"`
	Page     int                       `json:"page"`
	PageSize int                       `json:"pageSize"`
}
