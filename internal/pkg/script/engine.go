// Package script 提供一个内置的 JavaScript 代码解释器（基于 goja），
// 供数据迁移模块的管理员脚本使用：对接旧系统登录 API、校验旧密码格式、
// 转换导入数据字段等。
//
// 设计要点：
//   - goja.Runtime 非并发安全，因此每次执行新建 Runtime（或在单 goroutine
//     内复用，见 Engine.Run 注释）。
//   - 通过 context + time.AfterFunc + vm.Interrupt() 实现执行超时，避免
//     管理员脚本里的死循环挂死请求/导入任务。
//   - 注入 http / passhash / log / console 等便利对象。能力不额外做沙箱限制
//     （仅 super_admin 可写脚本），但保留超时这一最后防线。
package script

import (
	"context"
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/internal/pkg/passhash"
	"github.com/dop251/goja"
	"golang.org/x/crypto/bcrypt"
)

// DefaultTimeout 脚本默认执行超时。
const DefaultTimeout = 10 * time.Second

// Engine 是 goja 脚本执行器的薄封装。
type Engine struct {
	timeout time.Duration
}

// NewEngine 创建脚本引擎。timeout<=0 时使用 DefaultTimeout。
func NewEngine(timeout time.Duration) *Engine {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	return &Engine{timeout: timeout}
}

// Run 执行脚本，把 input 作为全局变量 `input` 注入，返回脚本的求值结果。
//
// 脚本约定：最后一个表达式的值即为返回值；也可写
//
//	return { success: true, user: {...} }
//
// 形式 —— 引擎会把脚本体包裹进一个匿名函数中执行，因此 return 合法。
//
// 返回的结果会被规整为 Go 原生类型（map[string]any / []any / 标量）。
func (e *Engine) Run(ctx context.Context, scriptSrc string, input map[string]any) (any, error) {
	vm := goja.New()
	vm.SetFieldNameMapper(goja.TagFieldNameMapper("json", true))

	// 超时中断：到点调用 vm.Interrupt，goja.Run 会返回 *InterruptedError。
	timer := time.AfterFunc(e.timeout, func() {
		vm.Interrupt("script execution timed out")
	})
	defer timer.Stop()

	// 调用方 context 取消时也中断。
	if ctx != nil && ctx.Done() != nil {
		stop := make(chan struct{})
		defer close(stop)
		go func() {
			select {
			case <-ctx.Done():
				vm.Interrupt(ctx.Err())
			case <-stop:
			}
		}()
	}

	if err := e.installGlobals(vm, input); err != nil {
		return nil, err
	}

	// 包裹进匿名函数，允许脚本使用 return；同时把 input 作为形参传入。
	wrapped := "(function(input){\n" + scriptSrc + "\n})(input)"
	value, err := vm.RunString(wrapped)
	if err != nil {
		return nil, fmt.Errorf("脚本执行失败: %w", err)
	}
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
		return nil, nil
	}
	return value.Export(), nil
}

// RunForMap 执行脚本并要求结果为对象，返回 map[string]any。
func (e *Engine) RunForMap(ctx context.Context, scriptSrc string, input map[string]any) (map[string]any, error) {
	out, err := e.Run(ctx, scriptSrc, input)
	if err != nil {
		return nil, err
	}
	if out == nil {
		return map[string]any{}, nil
	}
	m, ok := out.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("脚本返回值必须是对象，实际为 %T", out)
	}
	return m, nil
}

// installGlobals 注入 input / http / passhash / log / console 全局对象。
func (e *Engine) installGlobals(vm *goja.Runtime, input map[string]any) error {
	if input == nil {
		input = map[string]any{}
	}
	if err := vm.Set("input", input); err != nil {
		return err
	}
	if err := vm.Set("http", e.httpObject(vm)); err != nil {
		return err
	}
	if err := vm.Set("passhash", e.passhashObject(vm)); err != nil {
		return err
	}

	logFn := func(call goja.FunctionCall) goja.Value { return goja.Undefined() }
	logObj := vm.NewObject()
	_ = logObj.Set("info", logFn)
	_ = logObj.Set("error", logFn)
	_ = logObj.Set("warn", logFn)
	if err := vm.Set("log", logObj); err != nil {
		return err
	}
	consoleObj := vm.NewObject()
	_ = consoleObj.Set("log", logFn)
	_ = consoleObj.Set("error", logFn)
	_ = consoleObj.Set("warn", logFn)
	return vm.Set("console", consoleObj)
}

// httpObject 暴露 http.request({method,url,headers,body,timeoutMs})。
func (e *Engine) httpObject(vm *goja.Runtime) *goja.Object {
	obj := vm.NewObject()
	_ = obj.Set("request", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(vm.ToValue("http.request 需要一个配置对象参数"))
		}
		opts, ok := call.Argument(0).Export().(map[string]any)
		if !ok {
			panic(vm.ToValue("http.request 参数必须是对象"))
		}

		method := strings.ToUpper(asString(opts["method"]))
		if method == "" {
			method = http.MethodGet
		}
		url := asString(opts["url"])
		if url == "" {
			panic(vm.ToValue("http.request 缺少 url"))
		}

		var bodyReader io.Reader
		if b, ok := opts["body"]; ok && b != nil {
			switch v := b.(type) {
			case string:
				bodyReader = strings.NewReader(v)
			default:
				raw, err := json.Marshal(v)
				if err != nil {
					panic(vm.ToValue("http.request body 序列化失败: " + err.Error()))
				}
				bodyReader = strings.NewReader(string(raw))
			}
		}

		req, err := http.NewRequest(method, url, bodyReader)
		if err != nil {
			panic(vm.ToValue("http.request 构造请求失败: " + err.Error()))
		}
		if headers, ok := opts["headers"].(map[string]any); ok {
			for k, v := range headers {
				req.Header.Set(k, asString(v))
			}
		}

		timeout := 15 * time.Second
		if ms, ok := toFloat(opts["timeoutMs"]); ok && ms > 0 {
			timeout = time.Duration(ms) * time.Millisecond
		}
		client := &http.Client{Timeout: timeout}

		resp, err := client.Do(req)
		if err != nil {
			panic(vm.ToValue("http.request 请求失败: " + err.Error()))
		}
		defer resp.Body.Close()

		respBody, _ := io.ReadAll(resp.Body)
		result := map[string]any{
			"status": resp.StatusCode,
			"body":   string(respBody),
		}
		var parsed any
		if json.Unmarshal(respBody, &parsed) == nil {
			result["json"] = parsed
		}
		return vm.ToValue(result)
	})
	return obj
}

// passhashObject 构造注入脚本的全局 `passhash` 对象，提供常见哈希、HMAC、
// 密码校验与编码工具，便于迁移脚本校验旧系统的密码格式。
//
// 取名 passhash 与后端 Go 包 internal/pkg/passhash 一致，前后端命名统一、便于排查。
//
// 在迁移脚本（login_script / transform_script）里可直接用全局 `passhash`：
//
// 哈希（返回小写十六进制字符串）：
//   - passhash.md5(s)            => string   例: passhash.md5(input.password)
//   - passhash.sha1(s)           => string
//   - passhash.sha256(s)         => string   例: passhash.sha256(input.password + lu.salt)
//   - passhash.hmacSha256(key,s) => string   HMAC-SHA256，返回十六进制
//
// 密码校验/生成（推荐用于密码场景）：
//   - passhash.passVerify(hash, password) => bool
//     用统一密码库(gopasshash)按前缀自动识别 argon2id / bcrypt / pbkdf2
//     等“标准编码哈希”并校验。旧系统若用这些算法存储，优先用它。
//     例: if (passhash.passVerify(lu.passwordHash, input.password)) { ... }
//   - passhash.bcryptCompare(hash, password) => bool
//     仅比对 bcrypt（$2a$/$2b$/$2y$）哈希；passVerify 已涵盖 bcrypt，
//     此方法保留用于明确只接受 bcrypt 的场景。
//   - passhash.passHash(password) => string
//     用 argon2id 生成新哈希；出错返回空串。一般无需在脚本里调用——
//     迁移成功后由系统用本次明文自动重哈希为 argon2id。
//
// 编码（解码失败返回空串）：
//   - passhash.hexEncode(s) / passhash.hexDecode(s)       => string
//   - passhash.base64Encode(s) / passhash.base64Decode(s) => string（标准 base64）
//
// 说明：
//   - 所有入参/返回均为 JS 字符串；哈希类方法把字符串按 UTF-8 字节计算。
//   - 自定义旧格式（如 sha256(password+salt)）用 passhash.sha256 自行拼接比较；
//     标准编码格式（argon2id/bcrypt/pbkdf2）用 passhash.passVerify。
func (e *Engine) passhashObject(vm *goja.Runtime) *goja.Object {
	obj := vm.NewObject()

	// passhash.md5(s) => 十六进制字符串
	_ = obj.Set("md5", func(s string) string {
		sum := md5.Sum([]byte(s))
		return hex.EncodeToString(sum[:])
	})
	// passhash.sha1(s) => 十六进制字符串
	_ = obj.Set("sha1", func(s string) string {
		sum := sha1.Sum([]byte(s))
		return hex.EncodeToString(sum[:])
	})
	// passhash.sha256(s) => 十六进制字符串
	_ = obj.Set("sha256", func(s string) string {
		sum := sha256.Sum256([]byte(s))
		return hex.EncodeToString(sum[:])
	})
	// passhash.hmacSha256(key, s) => 十六进制字符串
	_ = obj.Set("hmacSha256", func(key, s string) string {
		mac := hmac.New(sha256.New, []byte(key))
		mac.Write([]byte(s))
		return hex.EncodeToString(mac.Sum(nil))
	})
	// passhash.bcryptCompare(hash, password) => bool，仅比对 bcrypt 哈希
	_ = obj.Set("bcryptCompare", func(hash, password string) bool {
		return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
	})
	// passhash.passVerify(hash, password) => bool
	// 用统一密码库自动识别 argon2id / bcrypt / pbkdf2 等标准编码哈希并校验。
	_ = obj.Set("passVerify", func(hash, password string) bool {
		return passhash.Verify(password, hash)
	})
	// passhash.passHash(password) => string，用 argon2id 生成新哈希（出错返回 ""）
	_ = obj.Set("passHash", func(password string) string {
		h, err := passhash.Hash(password)
		if err != nil {
			return ""
		}
		return h
	})
	// passhash.hexEncode(s) => 十六进制字符串
	_ = obj.Set("hexEncode", func(s string) string {
		return hex.EncodeToString([]byte(s))
	})
	// passhash.hexDecode(s) => 解码后字符串（非法输入返回 ""）
	_ = obj.Set("hexDecode", func(s string) string {
		b, _ := hex.DecodeString(s)
		return string(b)
	})
	// passhash.base64Encode(s) => 标准 base64 字符串
	_ = obj.Set("base64Encode", func(s string) string {
		return base64.StdEncoding.EncodeToString([]byte(s))
	})
	// passhash.base64Decode(s) => 解码后字符串（非法输入返回 ""）
	_ = obj.Set("base64Decode", func(s string) string {
		b, _ := base64.StdEncoding.DecodeString(s)
		return string(b)
	})
	return obj
}

func asString(v any) string {
	switch s := v.(type) {
	case nil:
		return ""
	case string:
		return s
	default:
		return fmt.Sprintf("%v", v)
	}
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int64:
		return float64(n), true
	case int:
		return float64(n), true
	default:
		return 0, false
	}
}
