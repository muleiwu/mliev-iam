// Package passhash 是全项目统一的密码哈希/校验入口，基于可插拔密码库
// github.com/muleiwu/gopasshash。
//
//   - 新密码一律用 argon2id（gopasshash 推荐默认参数）生成。
//   - 校验时按哈希前缀自动分派，识别 argon2id / bcrypt / pbkdf2 等标准编码格式。
//   - 登录成功后可借 VerifyAndUpgrade 把旧算法（如 bcrypt）透明升级为 argon2id。
//
// 历史的 "sha256(password+salt)" 非标准格式不在 gopasshash 识别范围内，
// 由调用方（user_service）在 IsRecognized 返回 false 时自行回落处理。
package passhash

import (
	"sync"

	gopasshash "github.com/muleiwu/gopasshash"
	"github.com/muleiwu/gopasshash/argon2id"
	"github.com/muleiwu/gopasshash/bcrypt"
	"github.com/muleiwu/gopasshash/pbkdf2"
)

var (
	once    sync.Once
	hasher  *gopasshash.Hasher
	initErr error
)

// get 懒构建单例 Hasher。target=argon2id，verifiers 额外识别 bcrypt / pbkdf2。
func get() (*gopasshash.Hasher, error) {
	once.Do(func() {
		hasher, initErr = gopasshash.New(
			argon2id.Default(),
			gopasshash.WithVerifiers(bcrypt.Default(), pbkdf2.Default()),
		)
	})
	return hasher, initErr
}

// Hash 用 argon2id 生成编码哈希。
func Hash(password string) (string, error) {
	h, err := get()
	if err != nil {
		return "", err
	}
	return h.Hash(password)
}

// Verify 校验密码与编码哈希是否匹配（按前缀自动分派）。
// 哈希格式不被识别或出错时返回 false。
func Verify(password, encodedHash string) bool {
	h, err := get()
	if err != nil {
		return false
	}
	ok, err := h.Verify(password, encodedHash)
	return err == nil && ok
}

// VerifyAndUpgrade 校验密码；匹配且源算法/参数与 target(argon2id) 不一致时，
// 返回升级后的新哈希。newHash != "" 表示调用方应持久化该新哈希。
// 校验失败或哈希不被识别时返回 (false, "")。
func VerifyAndUpgrade(password, encodedHash string) (match bool, newHash string) {
	h, err := get()
	if err != nil {
		return false, ""
	}
	res, err := h.VerifyAndUpgrade(password, encodedHash)
	if err != nil || !res.Match {
		return false, ""
	}
	if res.NeedsRehash {
		return true, res.NewHash
	}
	return true, ""
}

// IsRecognized 报告该哈希是否为 gopasshash 可识别的标准编码格式
// （argon2id / bcrypt / pbkdf2）。用于区分标准格式与历史 sha256+salt。
func IsRecognized(encodedHash string) bool {
	h, err := get()
	if err != nil {
		return false
	}
	_, err = h.Inspect(encodedHash)
	return err == nil
}
