package service

import "cnb.cool/mliev/iam/mliev-iam/app/model"

// OIDCSessionAutoApprovesConsent is evaluated after authentication and required
// profile completion. Only this transaction's login can replace confirmation.
func OIDCSessionAutoApprovesConsent(app *model.Application, session *OIDCSession) bool {
	if app == nil || session == nil || !session.IsAuthenticated || session.UserID == nil || session.PendingTwoFactorID != "" || session.Error != nil {
		return false
	}
	if ApplicationAutoApprovesConsent(app) {
		return true
	}
	return session.HasLoginInteraction && !oidcSessionPromptHas(session.Prompt, "consent")
}
