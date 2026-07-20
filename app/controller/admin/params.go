package admin

import (
	"fmt"
	"strconv"

	httpInterfaces "cnb.cool/mliev/open/go-web/pkg/server/http_server/interfaces"
)

func parseUintParam(c httpInterfaces.RouterContextInterface, name string) (uint, error) {
	value, err := strconv.ParseUint(c.Param(name), 10, 64)
	if err != nil || value == 0 {
		return 0, fmt.Errorf("参数 %s 必须是正整数", name)
	}
	return uint(value), nil
}
