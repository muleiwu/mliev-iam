package utils

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/nyaruka/phonenumbers"
)

// 登录/注册相关的标识符校验工具。
//
// 关键约束：用户账户(username) 禁止全数字、禁止包含 "@"。
// 因此登录时可无歧义地按输入判断标识符类型：
//   - 纯数字且命中手机号正则 → 手机号
//   - 含 "@" → 邮箱
//   - 其余 → 用户名

var (
	// usernameCharsetRegex 用户名允许的字符集：字母、数字、下划线（天然排除 "@"）。
	usernameCharsetRegex = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)
	// allDigitsRegex 纯数字。
	allDigitsRegex = regexp.MustCompile(`^[0-9]+$`)
	// e164Regex E.164 国际手机号格式：以 "+" 开头，首位非 0，总长 7-15 位数字。
	e164Regex = regexp.MustCompile(`^\+[1-9]\d{6,14}$`)
)

// DefaultPhoneRegion 解析无国家码手机号时使用的默认地区（兜底，正常前端传 E.164）。
const DefaultPhoneRegion = "CN"

// IsAllDigits 判断字符串是否为纯数字（非空）。
func IsAllDigits(s string) bool {
	return allDigitsRegex.MatchString(s)
}

// ValidateUsernameFormat 校验用户账户格式：
// 长度 3-50、仅含字母/数字/下划线、且不能为纯数字。
func ValidateUsernameFormat(username string) error {
	if len(username) < 3 || len(username) > 50 {
		return fmt.Errorf("账户长度需在3-50个字符之间")
	}
	if !usernameCharsetRegex.MatchString(username) {
		return fmt.Errorf("账户只能包含字母、数字和下划线")
	}
	if IsAllDigits(username) {
		return fmt.Errorf("账户不能为纯数字")
	}
	return nil
}

// LoginIdentifierKind 登录标识符类型。
type LoginIdentifierKind string

const (
	LoginIdentifierPhone    LoginIdentifierKind = "phone"
	LoginIdentifierEmail    LoginIdentifierKind = "email"
	LoginIdentifierUsername LoginIdentifierKind = "username"
)

// DetectLoginIdentifier 根据输入判断登录标识符类型。
// 与前端 utils/validators.ts 的判断规则保持一致：
//   - 以 "+" 开头且为合法 E.164 → 手机号
//   - 含 "@" → 邮箱
//   - 其余 → 用户名（username 字符集天然排除 "+"/"@"，无歧义）
func DetectLoginIdentifier(input string) LoginIdentifierKind {
	value := strings.TrimSpace(input)
	if IsValidE164(value) {
		return LoginIdentifierPhone
	}
	if strings.Contains(value, "@") {
		return LoginIdentifierEmail
	}
	return LoginIdentifierUsername
}

// IsValidE164 判断字符串是否为合法 E.164 手机号（如 +8613800138000）。
func IsValidE164(s string) bool {
	value := strings.TrimSpace(s)
	if !e164Regex.MatchString(value) {
		return false
	}
	num, err := phonenumbers.Parse(value, "")
	if err != nil {
		return false
	}
	return phonenumbers.IsValidNumber(num)
}

// NormalizePhoneToE164 把手机号规范化为 E.164 格式。
// 已是 "+" 前缀的按国际号解析；否则按 defaultRegion（空则用 DefaultPhoneRegion）解析。
func NormalizePhoneToE164(input, defaultRegion string) (string, error) {
	value := strings.TrimSpace(input)
	if value == "" {
		return "", fmt.Errorf("手机号不能为空")
	}
	region := defaultRegion
	if strings.HasPrefix(value, "+") {
		region = "" // 含国家码，无需默认地区
	} else if region == "" {
		region = DefaultPhoneRegion
	}
	num, err := phonenumbers.Parse(value, region)
	if err != nil {
		return "", fmt.Errorf("手机号格式不正确")
	}
	if !phonenumbers.IsValidNumber(num) {
		return "", fmt.Errorf("手机号格式不正确")
	}
	return phonenumbers.Format(num, phonenumbers.E164), nil
}
