// Package firstparty registers trusted clients at application composition time.
// Database flags alone cannot opt a client into same-origin callback handling.
package firstparty

import (
	"fmt"
	"net/url"
	"sync"
)

type Client struct{ ID, Name, Description, CallbackPath string }

const AdminID = "a1b2cb3c-af00-5ed4-b3da-f85b79012927"

var mu sync.RWMutex
var clients = map[string]Client{AdminID: {ID: AdminID, Name: "木雷坞 IAM 管理后台", Description: "IAM 系统内置管理后台", CallbackPath: "/api/admin/auth/callback"}}

// Register accepts compiled extension descriptors, never HTTP/database input.
func Register(c Client) {
	u, err := url.Parse(c.CallbackPath)
	if err != nil || c.ID == "" || c.Name == "" || u.IsAbs() || u.Host != "" || u.RawQuery != "" || u.Fragment != "" || len(u.Path) < 2 || u.Path[0] != '/' {
		panic("invalid first-party client descriptor")
	}
	mu.Lock()
	defer mu.Unlock()
	if old, ok := clients[c.ID]; ok && old != c {
		panic(fmt.Sprintf("conflicting first-party client %s", c.ID))
	}
	clients[c.ID] = c
}
func Lookup(id string) (Client, bool) {
	mu.RLock()
	defer mu.RUnlock()
	c, ok := clients[id]
	return c, ok
}
