package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type registrationCodeError struct{ kind VerificationType }

func (e *registrationCodeError) Error() string {
	return verificationTypeLabel(e.kind) + "验证码错误或已过期"
}

func (e *registrationCodeError) Unwrap() error { return ErrVerificationCodesInvalid }

// WithRegistrationCodes reserves all submitted proofs before running the database
// transaction. A failed registration restores the proofs with their original TTL;
// a committed registration consumes them. The random marker prevents concurrent
// reuse and remains unusable if cleanup fails or the process exits unexpectedly.
// SendCode may replace the marker with a newer code; finalization never changes it.
func (s *VerificationCodeService) WithRegistrationCodes(ctx context.Context, checks []VerificationCodeCheck, register func() error) (err error) {
	if len(checks) == 0 {
		return register()
	}
	id, err := uuid.NewRandom()
	if err != nil {
		return err
	}
	marker := "registration-reserved:" + id.String()
	keys := make([]string, 0, len(checks))
	reserveArgs := []interface{}{marker}
	for _, check := range checks {
		if (check.Type != VerificationTypeEmail && check.Type != VerificationTypePhone) || check.Receiver == "" || len(check.Code) != 6 {
			return &registrationCodeError{kind: check.Type}
		}
		keys = append(keys, s.getCodeKey(check.Receiver, check.Type, "register:"+check.Receiver))
		reserveArgs = append(reserveArgs, check.Code)
	}

	committed := false
	defer func() {
		// The HTTP request may be cancelled after reservation. Cleanup still has
		// a short independent deadline so ordinary failures remain retryable.
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		action := "restore"
		if committed {
			action = "consume"
		}
		args := []interface{}{marker, action}
		for _, check := range checks {
			args = append(args, check.Code)
		}
		const finalize = `for i, key in ipairs(KEYS) do
  if redis.call('GET', key) == ARGV[1] then
    if ARGV[2] == 'consume' then
      redis.call('DEL', key)
    else
      redis.call('SET', key, ARGV[i + 2], 'KEEPTTL')
    end
  end
end
return 1`
		if cleanupErr := s.redis.Eval(cleanupCtx, finalize, keys, args...).Err(); cleanupErr != nil {
			logVerificationCodeWarn("", "注册验证码结束处理失败 action=%s err=%v", action, cleanupErr)
			if !committed && err != nil {
				err = fmt.Errorf("%w；验证码暂无法恢复，请重新获取验证码后重试", err)
			}
		}
	}()

	// Validation and reservation are atomic across email and phone. A wrong code
	// leaves every proof untouched, and KEEPTTL never extends the validity window.
	const reserve = `for i, key in ipairs(KEYS) do
  if redis.call('GET', key) ~= ARGV[i + 1] or redis.call('PTTL', key) <= 0 then return i end
end
for _, key in ipairs(KEYS) do redis.call('SET', key, ARGV[1], 'KEEPTTL') end
return 0`
	failed, err := s.redis.Eval(ctx, reserve, keys, reserveArgs...).Int()
	if err != nil {
		logVerificationCodeWarn("", "注册验证码预留失败 err=%v", err)
		return errors.New("验证码服务暂不可用，请稍后重试")
	}
	if failed > 0 && failed <= len(checks) {
		return &registrationCodeError{kind: checks[failed-1].Type}
	}
	if failed != 0 {
		return ErrVerificationCodesInvalid
	}
	err = register()
	committed = err == nil
	return err
}
