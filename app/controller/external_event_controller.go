package controller

import (
	"io"
	"net/http"

	"cnb.cool/mliev/iam/mliev-iam/app/service"
	"cnb.cool/mliev/open/go-web/pkg/helper"
	httpInterfaces "cnb.cool/mliev/open/go-web/pkg/server/http_server/interfaces"
)

type ExternalEventController struct{}

func (ExternalEventController) Receive(c httpInterfaces.RouterContextInterface) {
	headers := c.Request().Header.Clone()
	copyCallbackQueryHeaders(c, headers)
	body, err := io.ReadAll(io.LimitReader(c.Request().Body, (4<<20)+1))
	if err != nil || len(body) > 4<<20 {
		c.JSON(http.StatusBadRequest, map[string]any{"error": "invalid event body"})
		return
	}
	result, err := service.NewExternalEventService(helper.GetDatabase()).Receive(c.Request().Context(), c.Param("identifier"), headers, body)
	if err != nil {
		c.JSON(http.StatusUnauthorized, map[string]any{"error": err.Error()})
		return
	}
	if len(result.ResponseBody) > 0 {
		contentType := result.ResponseContentType
		if contentType == "" {
			contentType = "text/plain; charset=utf-8"
		}
		c.Data(http.StatusOK, contentType, result.ResponseBody)
		return
	}
	if result.Challenge != "" {
		c.JSON(http.StatusOK, map[string]any{"challenge": result.Challenge})
		return
	}
	c.JSON(http.StatusOK, map[string]any{"code": 0, "duplicate": result.Duplicate})
}

func copyCallbackQueryHeaders(c httpInterfaces.RouterContextInterface, headers http.Header) {
	queryMappings := map[string]string{
		"msg_signature": "X-Callback-Signature",
		"signature":     "X-Callback-Signature",
		"timestamp":     "X-Callback-Timestamp",
		"timeStamp":     "X-Callback-Timestamp",
		"nonce":         "X-Callback-Nonce",
		"echostr":       "X-Callback-Echo",
	}
	for query, header := range queryMappings {
		if value := c.Query(query); value != "" {
			headers.Set(header, value)
		}
	}
}
