package diagnostic

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"testing"

	"github.com/aws/smithy-go"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestErrorsDoNotExposeProviderDetails(t *testing.T) {
	secret := "secret-account-private-volume-https://private-cluster.invalid"
	for _, tt := range []struct {
		err  error
		want string
	}{
		{errors.New(secret), "request failed"},
		{fmt.Errorf("%s: %w", secret, context.DeadlineExceeded), "timed out"},
		{context.Canceled, "interrupted"},
		{&smithy.GenericAPIError{Code: "AccessDenied", Message: secret}, "permission denied"},
		{&smithy.GenericAPIError{Code: "ExpiredToken", Message: secret}, "credentials expired"},
		{&smithy.GenericAPIError{Code: "Throttling", Message: secret}, "throttled"},
		{&smithy.GenericAPIError{Code: secret, Message: secret}, "request failed"},
		{apierrors.NewForbidden(schema.GroupResource{Resource: "persistentvolumeclaims"}, secret, errors.New(secret)), "permission denied"},
		{apierrors.NewUnauthorized(secret), "authentication failed"},
		{&url.Error{Op: "Get", URL: secret, Err: errors.New(secret)}, "connection failed"},
	} {
		message := Message(tt.err)
		if !strings.Contains(strings.ToLower(message), tt.want) || strings.Contains(message, secret) {
			t.Errorf("unsafe or unhelpful message: %q", message)
		}
	}
	if Message(nil) != "" {
		t.Fatal("nil error has a message")
	}
}

func TestDiscoveryGuidanceMatchesTypedFailure(t *testing.T) {
	secret := "secret-account-https://private-cluster.invalid"
	for _, tt := range []struct {
		name           string
		err            error
		kind           Kind
		want           string
		permissionHint bool
	}{
		{"nil", nil, None, "", false},
		{"authentication", &smithy.GenericAPIError{Code: "AuthFailure", Message: secret}, AuthenticationFailed, "Authentication failed while discovering regions", false},
		{"wrapped expired", fmt.Errorf("%s: %w", secret, &smithy.GenericAPIError{Code: "ExpiredToken", Message: secret}), CredentialsExpired, "Credentials expired", false},
		{"permission", &smithy.GenericAPIError{Code: "UnauthorizedOperation", Message: secret}, PermissionDenied, "Region discovery was denied", true},
		{"throttling", &smithy.GenericAPIError{Code: "ThrottlingException", Message: secret}, Throttled, "Request throttled", false},
		{"deadline", context.DeadlineExceeded, TimedOut, "timed out", false},
		{"network timeout", &net.DNSError{Name: secret, IsTimeout: true}, TimedOut, "timed out", false},
		{"connection", &url.Error{Op: "Get", URL: secret, Err: errors.New(secret)}, ConnectionFailed, "Connection failed", false},
		{"interrupted", context.Canceled, Interrupted, "interrupted", false},
		{"untrusted text", errors.New("AccessDenied " + secret), Unknown, "Region discovery failed", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			message := AWSRegionDiscoveryMessage(tt.err)
			if Classify(tt.err) != tt.kind || !strings.Contains(message, tt.want) {
				t.Fatalf("kind=%v message=%q", Classify(tt.err), message)
			}
			if strings.Contains(message, "ec2:DescribeRegions") != tt.permissionHint || strings.Contains(message, "--regions") != tt.permissionHint {
				t.Fatalf("unrelated permission/region hint: %q", message)
			}
			if strings.Contains(message, secret) {
				t.Fatalf("provider details leaked: %q", message)
			}
		})
	}
}

func TestKubernetesTypedFailures(t *testing.T) {
	for _, tt := range []struct {
		err  error
		kind Kind
	}{
		{apierrors.NewTimeoutError("private-cluster", 1), TimedOut},
		{apierrors.NewServerTimeout(schema.GroupResource{Resource: "pods"}, "list", 1), TimedOut},
		{apierrors.NewTooManyRequests("private-cluster", 1), Throttled},
	} {
		if Classify(tt.err) != tt.kind || strings.Contains(Message(tt.err), "private-cluster") {
			t.Fatalf("kind=%v message=%q", Classify(tt.err), Message(tt.err))
		}
	}
}
