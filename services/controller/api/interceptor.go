package api

import (
	"context"
	"crypto/x509"
	"errors"
	"log"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
)

// contextKey is a private type to avoid collisions in context.
type contextKey string

const (
	spiffeIDContextKey contextKey = "spiffe-id"
	roleContextKey     contextKey = "spiffe-role"
)

// UnarySPIFFEInterceptor enforces SPIFFE identity on unary RPCs.
func UnarySPIFFEInterceptor(trustDomain string, allowedRoles ...string) grpc.UnaryServerInterceptor {
	roles := makeRoleSet(allowedRoles)
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {

		spiffeID, role, err := extractAndVerifySPIFFE(ctx, trustDomain, roles)
		if err != nil {
			return nil, err
		}

		ctx = context.WithValue(ctx, spiffeIDContextKey, spiffeID)
		ctx = context.WithValue(ctx, roleContextKey, role)

		return handler(ctx, req)
	}
}

// UnaryAuthInterceptor enforces SPIFFE identity on unary RPCs, with optional
// method-level bypass for bootstrap enrollment.
func UnaryAuthInterceptor(trustDomain string, unauthenticatedMethods map[string]struct{}, allowedRoles ...string) grpc.UnaryServerInterceptor {
	roles := makeRoleSet(allowedRoles)
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {

		if _, ok := unauthenticatedMethods[info.FullMethod]; ok {
			return handler(ctx, req)
		}

		spiffeID, role, err := extractAndVerifySPIFFE(ctx, trustDomain, roles)
		if err != nil {
			return nil, err
		}

		ctx = context.WithValue(ctx, spiffeIDContextKey, spiffeID)
		ctx = context.WithValue(ctx, roleContextKey, role)

		return handler(ctx, req)
	}
}

// StreamSPIFFEInterceptor enforces SPIFFE identity on streaming RPCs.
func StreamSPIFFEInterceptor(trustDomain string, allowedRoles ...string) grpc.StreamServerInterceptor {
	roles := makeRoleSet(allowedRoles)
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {

		spiffeID, role, err := extractAndVerifySPIFFE(ss.Context(), trustDomain, roles)
		if err != nil {
			return err
		}

		wrapped := &wrappedStream{
			ServerStream: ss,
			ctx: context.WithValue(
				context.WithValue(ss.Context(), spiffeIDContextKey, spiffeID),
				roleContextKey,
				role,
			),
		}

		return handler(srv, wrapped)
	}
}

// wrappedStream allows us to override Context().
type wrappedStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *wrappedStream) Context() context.Context {
	return w.ctx
}

// SPIFFEIDFromContext returns the SPIFFE ID from context.
func SPIFFEIDFromContext(ctx context.Context) (string, bool) {
	v := ctx.Value(spiffeIDContextKey)
	if v == nil {
		return "", false
	}
	id, ok := v.(string)
	return id, ok
}

// RoleFromContext returns the SPIFFE role from context.
func RoleFromContext(ctx context.Context) (string, bool) {
	v := ctx.Value(roleContextKey)
	if v == nil {
		return "", false
	}
	role, ok := v.(string)
	return role, ok
}

// extractAndVerifySPIFFE pulls the peer certificate from context and validates
// the SPIFFE ID and role.
func extractAndVerifySPIFFE(
	ctx context.Context,
	trustDomain string,
	allowedRoles map[string]struct{},
) (string, string, error) {
	p, ok := peer.FromContext(ctx)
	if !ok {
		return "", "", errors.New("missing peer information")
	}

	tlsInfo, ok := p.AuthInfo.(credentials.TLSInfo)
	if !ok {
		return "", "", errors.New("connection is not using TLS")
	}

	if len(tlsInfo.State.PeerCertificates) == 0 {
		return "", "", errors.New("no peer certificates presented")
	}

	cert := tlsInfo.State.PeerCertificates[0]
	logPeerTLS(cert)

	if len(cert.URIs) != 1 {
		return "", "", errors.New("exactly one SPIFFE ID is required")
	}

	uri := cert.URIs[0]

	if uri.Scheme != "spiffe" {
		return "", "", errors.New("SPIFFE ID must use spiffe:// scheme")
	}

	if uri.Host != trustDomain {
		return "", "", errors.New("SPIFFE trust domain mismatch")
	}

	path := strings.TrimPrefix(uri.Path, "/")
	parts := strings.Split(path, "/")
	if len(parts) != 2 {
		return "", "", errors.New("invalid SPIFFE path format")
	}

	role := parts[0]
	if len(allowedRoles) > 0 {
		if _, ok := allowedRoles[role]; !ok {
			return "", "", errors.New("invalid SPIFFE role")
		}
	}

	return uri.String(), role, nil
}

func makeRoleSet(roles []string) map[string]struct{} {
	if len(roles) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(roles))
	for _, r := range roles {
		if r == "" {
			continue
		}
		set[r] = struct{}{}
	}
	return set
}

func logPeerTLS(cert *x509.Certificate) {
	if cert == nil {
		return
	}
	var spiffeURI string
	if len(cert.URIs) == 1 {
		spiffeURI = cert.URIs[0].String()
	}
	log.Printf(
		"mtls peer: subject=%q serial=%s not_after=%s spiffe=%q",
		cert.Subject.String(),
		cert.SerialNumber.String(),
		cert.NotAfter.Format(time.RFC3339),
		spiffeURI,
	)
}
