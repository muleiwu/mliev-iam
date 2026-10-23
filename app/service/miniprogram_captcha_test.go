package service

import (
	"context"
	"errors"
	"testing"

	"cnb.cool/mliev/iam/mliev-iam/app/dto"
	"cnb.cool/mliev/iam/mliev-iam/app/model"
	"github.com/stretchr/testify/require"
)

type nativeCaptchaStub struct {
	appID, scope string
	fail         bool
	calls        int
}

func (c *nativeCaptchaStub) Params(_ context.Context, app *model.Application, scope string, _ dto.MiniProgramCaptchaParamsRequest, _ string) (*dto.CaptchaClientParamsResponse, error) {
	c.appID, c.scope = app.UUID, scope
	return &dto.CaptchaClientParamsResponse{Enabled: true, Provider: "tencent_miniprogram", SessionUuid: "test-session"}, nil
}
func (c *nativeCaptchaStub) Validate(_ context.Context, app *model.Application, scope string, _ *dto.CaptchaTicket, _ string) error {
	c.calls++
	if app.UUID != c.appID || scope != c.scope {
		return errors.New("scope mismatch")
	}
	if c.fail {
		return errors.New("captcha rejected")
	}
	return nil
}
func TestMiniProgramCaptchaUsesBoundTransactionBeforePasswordBinding(t *testing.T) {
	s, app, _, user, _ := miniFixture(t)
	ctx := context.Background()
	start := miniStart(t, s, app, "captcha")
	c := &nativeCaptchaStub{fail: true}
	s.SetCaptcha(c)
	params, err := s.CaptchaParams(ctx, app.UUID, start.TransactionToken, dto.MiniProgramCaptchaParamsRequest{Fingerprint: "test-installation"}, "192.0.2.1")
	require.NoError(t, err)
	require.True(t, params.Enabled)
	require.Equal(t, miniHash(app.UUID, start.TransactionToken), c.scope)
	require.NotEqual(t, start.TransactionToken, c.scope)
	_, err = s.CaptchaParams(ctx, "other-client", start.TransactionToken, dto.MiniProgramCaptchaParamsRequest{Fingerprint: "test"}, "192.0.2.1")
	require.ErrorIs(t, err, ErrMiniTransaction)
	_, err = s.Bind(ctx, app.UUID, start.TransactionToken, user.Username, "old-password", "192.0.2.1")
	require.ErrorContains(t, err, "captcha rejected")
	var count int64
	require.NoError(t, s.db.Model(&model.ExternalPrincipal{}).Count(&count).Error)
	require.Zero(t, count)
	c.fail = false
	result, err := s.Bind(ctx, app.UUID, start.TransactionToken, user.Username, "old-password", "192.0.2.1")
	require.NoError(t, err)
	require.Equal(t, "consent_required", result.State)
	_, err = s.CaptchaParams(ctx, app.UUID, start.TransactionToken, dto.MiniProgramCaptchaParamsRequest{Fingerprint: "test"}, "192.0.2.1")
	require.ErrorIs(t, err, ErrMiniTransaction)
}
