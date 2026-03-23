# Workspace CA Control-Plane Notes

This branch changes how workspace-scoped connector and agent certificates are used so workspace enrollment does not break controller connectivity.

## Problem

When a connector or agent enrolled with a workspace-scoped certificate, the issued workload CA could differ from the controller's server CA.

That created two separate trust concerns:

- Controller transport trust:
  The connector or agent must verify the controller gRPC server certificate against the controller CA.
- Workload trust:
  The connector and agent must keep using the workspace CA for workload identity, workload renewal validation, and connector-to-agent mTLS.

If those two trust roots are treated as the same CA, workspace enrollment can succeed and then the control-plane stream or renewal can fail.

## Fixes

### 1. Separate controller trust from workload trust

Connector and agent runtime code now keeps these roles separate:

- `cfg.ca_pem` is used for controller-facing transport verification.
- `result.ca_pem` is used as the workload/workspace CA for renewal validation and workload-facing trust.

This applies to:

- connector control-plane connections to the controller
- connector renewal
- agent renewal

### 2. Present the full workload chain

Workspace-issued workload certificates are signed by a workspace intermediate CA.

To support controller-side client certificate verification, the connector and agent now retain and present the full chain:

- leaf workload certificate
- workspace intermediate certificate(s) returned by enrollment

The in-memory certificate stores now carry both:

- leaf certificate DER
- full certificate chain DER

### 3. Preserve workspace enrollment semantics

Workspace-scoped identities are expected to remain on the workspace CA during:

- initial enrollment
- renewal
- re-enrollment with workspace-bound tokens

The controller-side enrollment path in this branch is intended to preserve the workspace trust domain instead of unnecessarily falling back to the global trust domain.

## Operational Result

For a workspace like `asdf.zerotrust.com`:

- connector enrollment should produce:
  `spiffe://asdf.zerotrust.com/connector/<id>`
- agent enrollment should produce:
  `spiffe://asdf.zerotrust.com/agent/<id>`

At the same time:

- controller-facing mTLS continues to anchor on the controller CA
- connector-to-agent mTLS continues to use the workspace/workload CA path

## Validation

The Rust test suites were run after these updates:

- `services/connector`: passed
- `services/agent`: passed
