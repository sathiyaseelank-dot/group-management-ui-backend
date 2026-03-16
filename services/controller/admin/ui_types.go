package admin

import "controller/api"

type uiUser struct {
	ID                  string   `json:"id"`
	Name                string   `json:"name"`
	Type                string   `json:"type"`
	DisplayLabel        string   `json:"displayLabel"`
	Email               string   `json:"email"`
	Status              string   `json:"status"`
	Role                string   `json:"role"`
	Groups              []string `json:"groups"`
	CertificateIdentity string   `json:"certificateIdentity,omitempty"`
	CreatedAt           string   `json:"createdAt"`
}

type uiGroup struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Type             string `json:"type"`
	DisplayLabel     string `json:"displayLabel"`
	Description      string `json:"description"`
	MemberCount      int    `json:"memberCount"`
	ResourceCount    int    `json:"resourceCount"`
	TrustedProfileID string `json:"trustedProfileId,omitempty"`
	CreatedAt        string `json:"createdAt"`
	UpdatedAt        string `json:"updatedAt"`
}

type uiGroupMember struct {
	UserID   string `json:"userId"`
	UserName string `json:"userName"`
	Email    string `json:"email"`
}

type uiResource struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	Type           string  `json:"type"`
	Address        string  `json:"address"`
	Protocol       string  `json:"protocol"`
	PortFrom       *int    `json:"portFrom"`
	PortTo         *int    `json:"portTo"`
	Alias          *string `json:"alias,omitempty"`
	Description    string  `json:"description"`
	RemoteNetwork  *string `json:"remoteNetworkId,omitempty"`
	FirewallStatus string  `json:"firewallStatus"`
}

type uiAccessRule struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	ResourceID    string   `json:"resourceId"`
	AllowedGroups []string `json:"allowedGroups"`
	Enabled       bool     `json:"enabled"`
	CreatedAt     string   `json:"createdAt"`
	UpdatedAt     string   `json:"updatedAt"`
}

type uiConnector struct {
	ID                string  `json:"id"`
	Name              string  `json:"name"`
	Status            string  `json:"status"`
	Version           string  `json:"version"`
	Hostname          string  `json:"hostname"`
	RemoteNetworkID   string  `json:"remoteNetworkId"`
	LastSeen          string  `json:"lastSeen"`
	Installed         bool    `json:"installed"`
	LastPolicyVersion int     `json:"lastPolicyVersion"`
	LastSeenAt        *string `json:"lastSeenAt"`
	PrivateIP         string  `json:"privateIp"`
	Revoked           bool    `json:"revoked"`
}

type uiRemoteNetwork struct {
	ID                   string `json:"id"`
	Name                 string `json:"name"`
	Location             string `json:"location"`
	ConnectorCount       int    `json:"connectorCount"`
	OnlineConnectorCount int    `json:"onlineConnectorCount"`
	ResourceCount        int    `json:"resourceCount"`
	CreatedAt            string `json:"createdAt"`
	UpdatedAt            string `json:"updatedAt"`
}

type uiConnectorLog struct {
	ID        int    `json:"id"`
	Timestamp string `json:"timestamp"`
	Message   string `json:"message"`
}

type uiAgent struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	Status          string  `json:"status"`
	Version         string  `json:"version"`
	Hostname        string  `json:"hostname"`
	RemoteNetworkID string  `json:"remoteNetworkId"`
	ConnectorID     string  `json:"connectorId"`
	ConnectorName   string  `json:"connectorName"`
	Revoked         bool    `json:"revoked"`
	Installed       bool    `json:"installed"`
	LastSeen        string  `json:"lastSeen"`
	LastSeenAt      *string `json:"lastSeenAt"`
}

type uiServiceAccount struct {
	ID                      string `json:"id"`
	Name                    string `json:"name"`
	Type                    string `json:"type"`
	DisplayLabel            string `json:"displayLabel"`
	Status                  string `json:"status"`
	AssociatedResourceCount int    `json:"associatedResourceCount"`
	CreatedAt               string `json:"createdAt"`
}

type uiSubject struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Type         string `json:"type"`
	DisplayLabel string `json:"displayLabel"`
}

type uiTrustedProfile struct {
	ID                    string `json:"id"`
	WorkspaceID           string `json:"workspaceId"`
	Name                  string `json:"name"`
	RequireFirewall       bool   `json:"requireFirewall"`
	RequireDiskEncryption bool   `json:"requireDiskEncryption"`
	RequireScreenLock     bool   `json:"requireScreenLock"`
	MinOSVersion          string `json:"minOsVersion,omitempty"`
	CreatedAt             string `json:"createdAt"`
	UpdatedAt             string `json:"updatedAt"`
}

type uiDevicePosture struct {
	DeviceID          string `json:"deviceId"`
	WorkspaceID       string `json:"workspaceId"`
	SPIFFEID          string `json:"spiffeId"`
	OSType            string `json:"osType"`
	OSVersion         string `json:"osVersion"`
	Hostname          string `json:"hostname"`
	FirewallEnabled   bool   `json:"firewallEnabled"`
	DiskEncrypted     bool   `json:"diskEncrypted"`
	ScreenLockEnabled bool   `json:"screenLockEnabled"`
	ClientVersion     string `json:"clientVersion"`
	CollectedAt       string `json:"collectedAt"`
	ReportedAt        string `json:"reportedAt"`
}

type uiDevice struct {
	DeviceID          string `json:"deviceId"`
	WorkspaceID       string `json:"workspaceId"`
	UserID            string `json:"userId"`
	OwnerName         string `json:"ownerName"`
	OwnerEmail        string `json:"ownerEmail"`
	DeviceName        string `json:"deviceName"`
	DeviceModel       string `json:"deviceModel"`
	DeviceMake        string `json:"deviceMake"`
	SerialNumber      string `json:"serialNumber"`
	SPIFFEID          string `json:"spiffeId"`
	OSType            string `json:"osType"`
	OSVersion         string `json:"osVersion"`
	Hostname          string `json:"hostname"`
	ClientVersion     string `json:"clientVersion"`
	FirewallEnabled   bool   `json:"firewallEnabled"`
	DiskEncrypted     bool   `json:"diskEncrypted"`
	ScreenLockEnabled bool   `json:"screenLockEnabled"`
	CollectedAt       string `json:"collectedAt"`
	ReportedAt        string `json:"reportedAt"`
}

type policyResource = api.PolicyResource
