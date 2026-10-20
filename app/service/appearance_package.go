package service

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"io/fs"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"github.com/gorilla/css/scanner"
	_ "golang.org/x/image/webp"
)

const (
	ThemePackageMaxZip      = 32 << 20
	ThemePackageMaxExpanded = 64 << 20
	ThemePackageMaxFiles    = 128
)

var (
	ErrThemePackage  = errors.New("主题包无效")
	ErrThemeConflict = errors.New("同名同版本主题已存在，请修改版本后导入")
	ErrThemeInUse    = errors.New("主题正在生效，请先切换其他风格并保存")
)

func packageError(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrThemePackage, fmt.Sprintf(format, args...))
}

var themeVersion = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$`)

func validateThemeMetadata(m *dto.ThemeMetadata) error {
	m.Name, m.Version, m.Author, m.Description = strings.TrimSpace(m.Name), strings.TrimSpace(m.Version), strings.TrimSpace(m.Author), strings.TrimSpace(m.Description)
	if m.Name == "" || len([]rune(m.Name)) > 100 || strings.IndexFunc(m.Name, unicode.IsControl) >= 0 {
		return packageError("主题名称须为 1–100 个字符")
	}
	if len(m.Version) > 32 || !themeVersion.MatchString(m.Version) {
		return packageError("版本须为语义版本，例如 1.0.0")
	}
	if len([]rune(m.Author)) > 100 || len([]rune(m.Description)) > 1000 || strings.ContainsRune(m.Author, 0) || strings.ContainsRune(m.Description, 0) {
		return packageError("作者或描述过长")
	}
	return nil
}

func decodeThemeJSON(data []byte, value any) error {
	if !utf8.Valid(data) {
		return packageError("JSON 必须为 UTF-8")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(value); err != nil {
		return packageError("JSON 格式错误: %v", err)
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return packageError("JSON 后包含额外内容")
	}
	return nil
}

func themeAppearanceJSON(config dto.AppearanceConfig) (json.RawMessage, error) {
	data, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}
	var object map[string]json.RawMessage
	if err = json.Unmarshal(data, &object); err != nil {
		return nil, err
	}
	delete(object, "customCss")
	delete(object, "sourceThemeId")
	return json.MarshalIndent(object, "", "  ")
}

func cleanPackagePath(name string) bool {
	return name != "" && utf8.ValidString(name) && strings.IndexFunc(name, unicode.IsControl) < 0 && !strings.ContainsAny(name, "\\:\x00") && !strings.HasPrefix(name, "/") && path.Clean(name) == name && name != "." && name != ".." && !strings.HasPrefix(name, "../")
}

// CSS escape processing follows CSS strings/identifiers, not JSON string escaping.
func decodeCSSEscapes(value string) string {
	var out strings.Builder
	for i := 0; i < len(value); {
		if value[i] != '\\' {
			out.WriteByte(value[i])
			i++
			continue
		}
		i++
		if i == len(value) {
			break
		}
		if value[i] == '\n' {
			i++
			continue
		}
		start := i
		for i < len(value) && i-start < 6 && strings.ContainsRune("0123456789abcdefABCDEF", rune(value[i])) {
			i++
		}
		if i > start {
			v, _ := strconv.ParseUint(value[start:i], 16, 32)
			r := rune(v)
			if r == 0 || r > utf8.MaxRune || (r >= 0xd800 && r <= 0xdfff) {
				r = utf8.RuneError
			}
			out.WriteRune(r)
			if i < len(value) && strings.ContainsRune(" \t\n\r\f", rune(value[i])) {
				i++
			}
		} else {
			out.WriteByte(value[i])
			i++
		}
	}
	return out.String()
}
func cssString(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "\"", "\\\"")
	value = strings.ReplaceAll(value, "\n", "\\a ")
	value = strings.ReplaceAll(value, "\r", "\\d ")
	return "\"" + value + "\""
}
func cssURLValue(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 && (value[0] == '\'' || value[0] == '"') && value[len(value)-1] == value[0] {
		value = value[1 : len(value)-1]
	}
	return decodeCSSEscapes(value)
}

// Token-based URL traversal preserves ordinary strings/comments and handles escaped url().
func rewriteThemeCSS(source string, visit func(string) (string, error)) (string, error) {
	if !utf8.ValidString(source) {
		return "", packageError("CSS 必须为 UTF-8")
	}
	s := scanner.New(source)
	var out strings.Builder
	var stack []string
	importString := false
	for {
		token := s.Next()
		if token.Type == scanner.TokenEOF {
			break
		}
		if token.Type == scanner.TokenError {
			return "", packageError("CSS 第 %d 行存在未结束的字符串或注释", token.Line)
		}
		value := token.Value
		if token.Type == scanner.TokenAtKeyword {
			importString = strings.EqualFold(decodeCSSEscapes(value), "@import")
		}
		isURL := token.Type == scanner.TokenURI
		isURLFunction := token.Type == scanner.TokenFunction && strings.EqualFold(decodeCSSEscapes(strings.TrimSuffix(value, "(")), "url")
		if isURL || isURLFunction {
			raw := value
			if isURLFunction {
				for {
					next := s.Next()
					if next.Type == scanner.TokenEOF || next.Type == scanner.TokenError {
						return "", packageError("CSS url() 未结束")
					}
					raw += next.Value
					if next.Type == scanner.TokenChar && next.Value == ")" {
						break
					}
				}
			}
			original := cssURLValue(raw[strings.Index(raw, "(")+1 : len(raw)-1])
			mapped, err := visit(original)
			if err != nil {
				return "", err
			}
			if mapped == original {
				out.WriteString(raw)
			} else {
				out.WriteString("url(" + cssString(mapped) + ")")
			}
			importString = false
			continue
		}
		imageSet := len(stack) > 0 && (stack[len(stack)-1] == "image-set" || stack[len(stack)-1] == "-webkit-image-set")
		if token.Type == scanner.TokenString && (importString || imageSet) {
			original := cssURLValue(value)
			mapped, err := visit(original)
			if err != nil {
				return "", err
			}
			if mapped != original {
				value = cssString(mapped)
			}
			importString = false
		}
		if token.Type == scanner.TokenFunction {
			stack = append(stack, strings.ToLower(decodeCSSEscapes(strings.TrimSuffix(value, "("))))
		}
		if token.Type == scanner.TokenChar {
			switch value {
			case "{", "[", "(":
				stack = append(stack, value)
			case "}", "]", ")":
				if len(stack) == 0 {
					return "", packageError("CSS 括号不匹配")
				}
				top := stack[len(stack)-1]
				if (value == "}" && top != "{") || (value == "]" && top != "[") || (value == ")" && (top == "{" || top == "[")) {
					return "", packageError("CSS 括号不匹配")
				}
				stack = stack[:len(stack)-1]
			case ";":
				importString = false
			}
		}
		out.WriteString(value)
	}
	if len(stack) > 0 {
		return "", packageError("CSS 括号未结束")
	}
	return out.String(), nil
}

type parsedThemePackage struct {
	Manifest dto.ThemeManifest
	Config   dto.AppearanceConfig
	Assets   map[string][]byte
}

func readThemeArchive(data []byte) (map[string][]byte, error) {
	if len(data) > ThemePackageMaxZip {
		return nil, packageError("ZIP 超过 32 MiB")
	}
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, packageError("无法读取 ZIP")
	}
	if len(z.File) > ThemePackageMaxFiles {
		return nil, packageError("文件数量超过 128")
	}
	files := map[string][]byte{}
	seen := map[string]bool{}
	total := int64(0)
	for _, f := range z.File {
		name := strings.TrimSuffix(f.Name, "/")
		if !cleanPackagePath(name) || seen[strings.ToLower(name)] {
			return nil, packageError("路径不安全或重复: %s", f.Name)
		}
		seen[strings.ToLower(name)] = true
		if f.Mode()&fs.ModeSymlink != 0 || (!f.FileInfo().IsDir() && !f.Mode().IsRegular()) || f.Flags&1 != 0 {
			return nil, packageError("不支持的 ZIP 条目: %s", f.Name)
		}
		if f.UncompressedSize64 > uint64(ThemePackageMaxExpanded-total) {
			return nil, packageError("解压内容超过 64 MiB")
		}
		if f.FileInfo().IsDir() {
			continue
		}
		reader, err := f.Open()
		if err != nil {
			return nil, packageError("无法读取 %s", name)
		}
		content, readErr := io.ReadAll(io.LimitReader(reader, ThemePackageMaxExpanded-total+1))
		reader.Close()
		if readErr != nil {
			return nil, packageError("文件校验失败: %s", name)
		}
		total += int64(len(content))
		if total > ThemePackageMaxExpanded {
			return nil, packageError("解压内容超过 64 MiB")
		}
		files[name] = content
	}
	if _, ok := files["theme.json"]; !ok {
		prefix := ""
		for name := range files {
			if path.Base(name) == "theme.json" && strings.Count(name, "/") == 1 {
				if prefix != "" {
					return nil, packageError("存在多个主题目录")
				}
				prefix = path.Dir(name) + "/"
			}
		}
		if prefix == "" {
			return nil, packageError("缺少 theme.json")
		}
		normalized := map[string][]byte{}
		for name, content := range files {
			if strings.HasPrefix(name, "__MACOSX/") || path.Base(name) == ".DS_Store" {
				continue
			}
			if !strings.HasPrefix(name, prefix) {
				return nil, packageError("主题文件必须位于同一个目录")
			}
			normalized[strings.TrimPrefix(name, prefix)] = content
		}
		files = normalized
	}
	return files, nil
}

func validateThemeImage(data []byte, mime string) error {
	var format string
	switch mime {
	case "image/png":
		format = "png"
	case "image/jpeg":
		format = "jpeg"
	case "image/webp":
		format = "webp"
	default:
		return packageError("仅支持 PNG、JPEG、WebP 图片")
	}
	config, kind, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || kind != format || config.Width <= 0 || config.Height <= 0 || config.Width > 8192 || config.Height > 8192 || int64(config.Width)*int64(config.Height) > 32_000_000 {
		return packageError("图片无效、格式不匹配或尺寸过大")
	}
	if _, _, err = image.Decode(bytes.NewReader(data)); err != nil {
		return packageError("图片内容不完整")
	}
	return nil
}

func parseThemePackage(data []byte) (*parsedThemePackage, error) {
	files, err := readThemeArchive(data)
	if err != nil {
		return nil, err
	}
	if len(files["theme.json"]) > 128*1024 {
		return nil, packageError("主题清单过大")
	}
	var manifest dto.ThemeManifest
	if err = decodeThemeJSON(files["theme.json"], &manifest); err != nil {
		return nil, err
	}
	if manifest.FormatVersion != 1 {
		return nil, packageError("不支持的主题包格式版本")
	}
	if err = validateThemeMetadata(&manifest.ThemeMetadata); err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(manifest.Appearance, &fields) != nil || fields == nil {
		return nil, packageError("缺少外观配置")
	}
	if _, ok := fields["customCss"]; ok {
		return nil, packageError("请将 CSS 保存到 theme.css")
	}
	if _, ok := fields["sourceThemeId"]; ok {
		return nil, packageError("主题包不能依赖其他已安装主题")
	}
	var config dto.AppearanceConfig
	if err = decodeThemeJSON(manifest.Appearance, &config); err != nil {
		return nil, err
	}
	css, ok := files["theme.css"]
	if !ok {
		return nil, packageError("缺少 theme.css")
	}
	if len(css) > 32*1024 {
		return nil, packageError("CSS 超过 32 KiB")
	}
	config.CustomCSS = string(css)
	assets := map[string][]byte{}
	for _, asset := range manifest.Assets {
		if !cleanPackagePath(asset.Path) || !strings.HasPrefix(asset.Path, "assets/") || assets[asset.Path] != nil {
			return nil, packageError("图片路径无效或重复: %s", asset.Path)
		}
		ext := strings.ToLower(path.Ext(asset.Path))
		if !((asset.MimeType == "image/png" && ext == ".png") || (asset.MimeType == "image/jpeg" && (ext == ".jpg" || ext == ".jpeg")) || (asset.MimeType == "image/webp" && ext == ".webp")) {
			return nil, packageError("图片扩展名与类型不匹配: %s", asset.Path)
		}
		data, ok := files[asset.Path]
		if !ok {
			return nil, packageError("缺少图片: %s", asset.Path)
		}
		if err := validateThemeImage(data, asset.MimeType); err != nil {
			return nil, fmt.Errorf("%s: %w", asset.Path, err)
		}
		assets[asset.Path] = data
	}
	p := &parsedThemePackage{Manifest: manifest, Config: config, Assets: assets}
	// Preflight expanded URLs before any uploads, including source association overhead.
	preview, err := p.rewrite(func(string) (string, error) { return "/attachments/00000000-0000-0000-0000-000000000000", nil })
	if err != nil {
		return nil, err
	}
	preview.SourceThemeID = "00000000-0000-0000-0000-000000000000"
	settings := dto.AppearanceSettings{Mode: "custom", Custom: preview}
	if err = ValidateAppearanceSettings(&settings); err != nil {
		return nil, packageError("%v", err)
	}
	// Canonical defaults belong to the imported configuration as well.
	config.Preset = settings.Custom.Preset
	config.DesktopAlignment = settings.Custom.DesktopAlignment
	config.BackgroundMode = settings.Custom.BackgroundMode
	p.Config = config
	return p, nil
}

func (p *parsedThemePackage) rewrite(assetURL func(string) (string, error)) (dto.AppearanceConfig, error) {
	config := p.Config
	visit := func(raw string) (string, error) {
		if raw == "" || strings.HasPrefix(raw, "#") {
			return raw, nil
		}
		u, err := url.Parse(raw)
		if err != nil {
			return "", packageError("资源地址无效: %s", raw)
		}
		if u.Scheme != "" || u.Host != "" {
			if u.Scheme == "http" || u.Scheme == "https" || u.Scheme == "data" || u.Host != "" {
				return raw, nil
			}
			return "", packageError("不支持的资源协议")
		}
		name := strings.TrimPrefix(u.Path, "./")
		if !cleanPackagePath(name) || !strings.HasPrefix(name, "assets/") || p.Assets[name] == nil {
			return "", packageError("资源必须使用声明的 assets/ 路径或外部地址: %s", raw)
		}
		mapped, err := assetURL(name)
		if err != nil {
			return "", err
		}
		if u.RawQuery != "" {
			mapped += "?" + u.RawQuery
		}
		if u.Fragment != "" {
			mapped += "#" + u.Fragment
		}
		return mapped, nil
	}
	var err error
	if config.Logo != "" {
		config.Logo, err = visit(config.Logo)
		if err != nil {
			return config, err
		}
	}
	if config.BackgroundImage != "" {
		config.BackgroundImage, err = visit(config.BackgroundImage)
		if err != nil {
			return config, err
		}
	}
	config.CustomCSS, err = rewriteThemeCSS(config.CustomCSS, visit)
	return config, err
}

func writeThemeZip(files map[string][]byte) ([]byte, error) {
	if len(files) > ThemePackageMaxFiles {
		return nil, packageError("导出文件数量超过限制")
	}
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	names := make([]string, 0, len(files))
	total := 0
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if !cleanPackagePath(name) {
			return nil, packageError("无效导出路径")
		}
		data := files[name]
		total += len(data)
		if total > ThemePackageMaxExpanded {
			return nil, packageError("导出内容超过 64 MiB")
		}
		h := &zip.FileHeader{Name: name, Method: zip.Deflate}
		h.SetMode(0644)
		w, err := z.CreateHeader(h)
		if err != nil {
			return nil, err
		}
		if _, err = w.Write(data); err != nil {
			return nil, err
		}
	}
	if err := z.Close(); err != nil {
		return nil, err
	}
	if buf.Len() > ThemePackageMaxZip {
		return nil, packageError("导出包超过 32 MiB")
	}
	return buf.Bytes(), nil
}
