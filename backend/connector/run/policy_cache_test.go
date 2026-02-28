package run

import (
	"testing"
	"time"
)

const testKey = "test-signing-key"

func newSignedSnapshot(t *testing.T, resources []policyResource) policySnapshot {
	t.Helper()
	snap := policySnapshot{
		SnapshotMeta: snapshotMeta{
			ConnectorID:   "con_test",
			PolicyVersion: 1,
			CompiledAt:    time.Now().UTC().Format(time.RFC3339),
			ValidUntil:    time.Now().UTC().Add(10 * time.Minute).Format(time.RFC3339),
			Signature:     "",
		},
		Resources: resources,
	}
	sig, err := signSnapshot([]byte(testKey), snap)
	if err != nil {
		t.Fatalf("signSnapshot failed: %v", err)
	}
	snap.SnapshotMeta.Signature = sig
	return snap
}

func newCache(t *testing.T, resources []policyResource) *policyCache {
	t.Helper()
	cache := newPolicyCache([]byte(testKey), 5*time.Minute)
	if ok := cache.ReplaceSnapshot(newSignedSnapshot(t, resources)); !ok {
		t.Fatalf("ReplaceSnapshot failed")
	}
	return cache
}

func TestPolicyCacheDNSAllow(t *testing.T) {
	cache := newCache(t, []policyResource{
		{
			ResourceID:        "res_dns_allow",
			Type:              "dns",
			Address:           "db.internal",
			Protocol:          "TCP",
			AllowedIdentities: []string{"identity-1"},
		},
	})

	allowed, _, reason := cache.Allowed("identity-1", "db.internal", "TCP", 5432)
	if !allowed || reason != "allowed" {
		t.Fatalf("expected allow, got allowed=%v reason=%s", allowed, reason)
	}
}

func TestPolicyCacheDNSDeny(t *testing.T) {
	cache := newCache(t, []policyResource{
		{
			ResourceID:        "res_dns_deny",
			Type:              "dns",
			Address:           "db.internal",
			Protocol:          "TCP",
			AllowedIdentities: []string{"identity-1"},
		},
	})

	allowed, _, reason := cache.Allowed("identity-2", "db.internal", "TCP", 5432)
	if allowed || reason != "not_allowed" {
		t.Fatalf("expected deny not_allowed, got allowed=%v reason=%s", allowed, reason)
	}
}

func TestPolicyCacheCIDRAllow(t *testing.T) {
	cache := newCache(t, []policyResource{
		{
			ResourceID:        "res_cidr_allow",
			Type:              "cidr",
			Address:           "10.0.10.0/24",
			Protocol:          "TCP",
			AllowedIdentities: []string{"identity-1"},
		},
	})

	allowed, _, reason := cache.Allowed("identity-1", "10.0.10.50", "TCP", 443)
	if !allowed || reason != "allowed" {
		t.Fatalf("expected allow, got allowed=%v reason=%s", allowed, reason)
	}
}

func TestPolicyCacheCIDRNoMatchHostname(t *testing.T) {
	cache := newCache(t, []policyResource{
		{
			ResourceID:        "res_cidr_only",
			Type:              "cidr",
			Address:           "10.0.10.0/24",
			Protocol:          "TCP",
			AllowedIdentities: []string{"identity-1"},
		},
	})

	allowed, _, reason := cache.Allowed("identity-1", "db.internal", "TCP", 443)
	if allowed || reason != "resource_not_found" {
		t.Fatalf("expected resource_not_found, got allowed=%v reason=%s", allowed, reason)
	}
}

func TestPolicyCacheInternetFallback(t *testing.T) {
	cache := newCache(t, []policyResource{
		{
			ResourceID:        "res_internet",
			Type:              "internet",
			Address:           "*",
			Protocol:          "TCP",
			AllowedIdentities: []string{"identity-1"},
		},
	})

	allowed, _, reason := cache.Allowed("identity-1", "unknown.host", "TCP", 443)
	if !allowed || reason != "allowed" {
		t.Fatalf("expected allow, got allowed=%v reason=%s", allowed, reason)
	}
}

func TestPolicyCacheMultiResourceNoEarlyDeny(t *testing.T) {
	cache := newCache(t, []policyResource{
		{
			ResourceID:        "res_denied",
			Type:              "dns",
			Address:           "db.internal",
			Protocol:          "TCP",
			AllowedIdentities: []string{"identity-2"},
		},
		{
			ResourceID:        "res_allowed",
			Type:              "dns",
			Address:           "db.internal",
			Protocol:          "TCP",
			AllowedIdentities: []string{"identity-1"},
		},
	})

	allowed, resourceID, reason := cache.Allowed("identity-1", "db.internal", "TCP", 5432)
	if !allowed || reason != "allowed" || resourceID != "res_allowed" {
		t.Fatalf("expected allow on res_allowed, got allowed=%v resourceID=%s reason=%s", allowed, resourceID, reason)
	}
}
