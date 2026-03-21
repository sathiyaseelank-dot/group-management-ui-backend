package state

import (
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log"
	"time"
)

// AuditEntry represents a single audit log record.
type AuditEntry struct {
	Timestamp   int64
	Actor       string // email or system identifier
	Action      string // e.g. "resource.create", "user.delete"
	Target      string // e.g. resource ID
	WorkspaceID string
	IPAddress   string
	Result      string // "ok" or error description
	Signature   string // HMAC-SHA256 of the entry
}

// WriteAudit inserts an audit log entry with HMAC tamper protection.
func WriteAudit(db *sql.DB, hmacKey []byte, entry AuditEntry) {
	if db == nil {
		return
	}
	if entry.Timestamp == 0 {
		entry.Timestamp = time.Now().Unix()
	}
	// Compute HMAC signature over entry fields
	entry.Signature = computeAuditHMAC(hmacKey, entry)
	_, err := db.Exec(
		Rebind(`INSERT INTO admin_audit_logs (timestamp, actor, action, target, result, workspace_id, ip_address, signature)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`),
		entry.Timestamp, entry.Actor, entry.Action, entry.Target, entry.Result,
		entry.WorkspaceID, entry.IPAddress, entry.Signature,
	)
	if err != nil {
		log.Printf("audit: write failed: %v", err)
	}
}

func computeAuditHMAC(key []byte, e AuditEntry) string {
	if len(key) == 0 {
		return ""
	}
	mac := hmac.New(sha256.New, key)
	fmt.Fprintf(mac, "%d|%s|%s|%s|%s|%s|%s",
		e.Timestamp, e.Actor, e.Action, e.Target, e.Result, e.WorkspaceID, e.IPAddress)
	return hex.EncodeToString(mac.Sum(nil))
}

// PolicyAuditEntry represents a policy evaluation audit log record.
type PolicyAuditEntry struct {
	Timestamp     int64
	Actor         string // SPIFFE ID of the principal
	Action        string // always "policy_evaluation"
	Target        string // resource ID
	WorkspaceID   string
	IPAddress     string
	Result        string // "allowed" or "denied"
	PolicyRuleID  string // ID of the matching policy rule (empty if none)
	PolicyDecision string // "allowed" or "denied"
	PolicyReason   string // explanation of the policy decision
	Signature     string // HMAC-SHA256 of the entry
}

// WritePolicyAudit inserts a policy evaluation audit log entry with HMAC tamper protection.
// This includes policy_rule_id, policy_decision, and policy_reason fields.
func WritePolicyAudit(db *sql.DB, hmacKey []byte, entry PolicyAuditEntry) {
	if db == nil {
		return
	}
	if entry.Timestamp == 0 {
		entry.Timestamp = time.Now().Unix()
	}
	// Compute HMAC signature over entry fields
	entry.Signature = computePolicyAuditHMAC(hmacKey, entry)
	_, err := db.Exec(
		Rebind(`INSERT INTO audit_logs (principal_spiffe, agent_id, resource_id, destination, protocol, port, decision, reason, connection_id, created_at, workspace_id, policy_rule_id, policy_decision, policy_reason)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		entry.Actor, entry.Target, entry.Target, "", "", 0, entry.Result, entry.PolicyReason, "",
		entry.Timestamp, entry.WorkspaceID, entry.PolicyRuleID, entry.PolicyDecision, entry.PolicyReason,
	)
	if err != nil {
		log.Printf("policy audit: write failed: %v", err)
	}
}

func computePolicyAuditHMAC(key []byte, e PolicyAuditEntry) string {
	if len(key) == 0 {
		return ""
	}
	mac := hmac.New(sha256.New, key)
	fmt.Fprintf(mac, "%d|%s|%s|%s|%s|%s|%s|%s|%s|%s",
		e.Timestamp, e.Actor, e.Action, e.Target, e.Result, e.WorkspaceID, e.IPAddress, e.PolicyRuleID, e.PolicyDecision, e.PolicyReason)
	return hex.EncodeToString(mac.Sum(nil))
}
