package controller

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"cnb.cool/mliev/iam/mliev-iam/app/constants"
	"cnb.cool/mliev/iam/mliev-iam/app/service"
	"cnb.cool/mliev/open/go-web/pkg/helper"
	httpInterfaces "cnb.cool/mliev/open/go-web/pkg/server/http_server/interfaces"
)

func (ec ExternalAuthController) MiniProgramIdentity(c httpInterfaces.RouterContextInterface) {
	c.SetHeader("Cache-Control", "no-store")
	cfg := helper.GetConfig()
	clientID, secret, ok := c.Request().BasicAuth()
	// Both ends default to disabled. A public application cannot opt itself in.
	if !cfg.GetBool("miniprogram.identity_exchange.enabled", false) || !ok || secret == "" || clientID == "" ||
		clientID != cfg.GetString("miniprogram.identity_exchange.client_id", "") ||
		c.Param("identifier") != cfg.GetString("miniprogram.identity_exchange.connection", "mp-main") {
		ec.Error(c, constants.ErrCodeUnauthorized, "小程序身份交换未启用或调用方无权限")
		return
	}
	var req struct {
		JsCode string `json:"jsCode" binding:"required,max=256"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		ec.Error(c, constants.ErrCodeBadRequest, "微信登录凭证不能为空")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request().Context(), 12*time.Second)
	defer cancel()
	redis := helper.GetRedis()
	if redis == nil {
		ec.Error(c, constants.ErrCodeInternal, "登录服务暂不可用")
		return
	}
	// Atomic rate limiting before an upstream request. No login codes enter Redis keys.
	key := fmt.Sprintf("miniprogram:identity:rate:%x", sha256.Sum256([]byte(clientID+":"+c.ClientIP())))
	count, err := redis.Eval(ctx, `local n=redis.call('INCR',KEYS[1]); if n==1 then redis.call('EXPIRE',KEYS[1],60) end; return n`, []string{key}).Int()
	if err != nil {
		ec.Error(c, constants.ErrCodeInternal, "登录服务暂不可用")
		return
	}
	if count > 120 {
		ec.Error(c, constants.ErrCodeTooManyRequests, "登录请求过于频繁")
		return
	}
	result, err := service.NewExternalAuthService(helper.GetDatabase(), redis).ResolveMiniProgramIdentity(ctx, c.Param("identifier"), req.JsCode, clientID, secret)
	if err != nil {
		ec.Error(c, constants.ErrCodeBadRequest, err.Error())
		return
	}
	ec.Success(c, result)
}
