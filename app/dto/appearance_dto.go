package dto

// AppearanceSettings keeps the custom draft when the built-in style is active.
type AppearanceSettings struct {
	Mode   string           `json:"mode"`
	Custom AppearanceConfig `json:"custom"`
}

type AppearanceConfig struct {
	BrandName       string `json:"brandName"`
	Logo            string `json:"logo"`
	PrimaryColor    string `json:"primaryColor"`
	DarkMode        string `json:"darkMode"`
	BackgroundMode  string `json:"backgroundMode"`
	BackgroundColor string `json:"backgroundColor"`
	BackgroundImage string `json:"backgroundImage"`
	CardRadius      *int   `json:"cardRadius"`
	ControlRadius   *int   `json:"controlRadius"`
	CustomCSS       string `json:"customCss"`
}

// PublicAppearanceResponse exposes only the active style, never an inactive draft.
type PublicAppearanceResponse struct {
	Mode       string           `json:"mode"`
	Appearance AppearanceConfig `json:"appearance"`
}
