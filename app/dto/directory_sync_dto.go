package dto

import "cnb.cool/mliev/iam/mliev-iam/app/model"

type DirectoryPolicyRequest struct {
	RootDepartmentID  uint                       `json:"rootDepartmentId" binding:"required"`
	SyncScope         map[string]any             `json:"syncScope"`
	FieldMapping      map[string]any             `json:"fieldMapping"`
	FieldOwnership    map[string]any             `json:"fieldOwnership"`
	MatchPolicy       model.DirectoryMatchPolicy `json:"matchPolicy"`
	DeprovisionPolicy string                     `json:"deprovisionPolicy"`
	Enabled           bool                       `json:"enabled"`
	Credentials       map[string]any             `json:"credentials"`
}

type DirectoryPolicyResponse struct {
	Policy               *model.DirectorySyncPolicy `json:"policy"`
	DirectoryCredentials bool                       `json:"directoryCredentialsConfigured"`
}

type DirectoryChangeDecisionRequest struct {
	Reason string `json:"reason"`
}
