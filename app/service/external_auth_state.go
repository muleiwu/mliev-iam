package service

const StateModeBind = "bind"

// StateData is the server-side OAuth transaction envelope. It is consumed
// atomically and never exposed as an authorization credential to the browser.
type StateData struct {
	SessionID            string `json:"sessionId,omitempty"`
	Mode                 string `json:"mode,omitempty"`
	BindUserID           uint   `json:"bindUserId,omitempty"`
	ConnectionIdentifier string `json:"connectionIdentifier,omitempty"`
	OpenerOrigin         string `json:"openerOrigin,omitempty"`
	BrowserBindingHash   string `json:"browserBindingHash,omitempty"`
	Timestamp            int64  `json:"timestamp"`
}

func (d *StateData) IsBind() bool { return d != nil && d.Mode == StateModeBind }
