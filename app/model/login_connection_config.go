package model

// LoginConnectionConfig controls entry-point presentation only, not authentication access.
type LoginConnectionConfig struct {
	Mode          string `json:"mode"`
	ConnectionIDs []uint `json:"connectionIds"`
}

const (
	LoginConnectionsInherit = "inherit"
	LoginConnectionsCustom  = "custom"
)

// LoginConnectionSettings supplies the historical default for applications without settings.
func (a *Application) LoginConnectionSettings() LoginConnectionConfig {
	result := LoginConnectionConfig{Mode: LoginConnectionsInherit, ConnectionIDs: []uint{}}
	if a != nil && a.LoginConnections != nil {
		result.Mode = a.LoginConnections.Mode
		result.ConnectionIDs = append(result.ConnectionIDs, a.LoginConnections.ConnectionIDs...)
	}
	return result
}
