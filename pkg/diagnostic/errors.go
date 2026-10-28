// Package diagnostic turns provider errors into messages safe to share publicly.
package diagnostic

import (
	"context"
	"errors"
	"net"

	"github.com/aws/smithy-go"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// Kind identifies a failure without retaining sensitive provider details.
type Kind uint8

const (
	None Kind = iota
	Unknown
	Interrupted
	TimedOut
	PermissionDenied
	AuthenticationFailed
	CredentialsExpired
	Throttled
	ConnectionFailed
)

// Classify inspects error types and allowlisted provider codes, never error text.
func Classify(err error) Kind {
	switch {
	case err == nil:
		return None
	case errors.Is(err, context.Canceled):
		return Interrupted
	case errors.Is(err, context.DeadlineExceeded):
		return TimedOut
	case apierrors.IsForbidden(err):
		return PermissionDenied
	case apierrors.IsUnauthorized(err):
		return AuthenticationFailed
	case apierrors.IsTimeout(err), apierrors.IsServerTimeout(err):
		return TimedOut
	case apierrors.IsTooManyRequests(err):
		return Throttled
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "AccessDenied", "AccessDeniedException", "UnauthorizedOperation", "Forbidden":
			return PermissionDenied
		case "ExpiredToken", "ExpiredTokenException", "RequestExpired", "TokenRefreshRequired":
			return CredentialsExpired
		case "InvalidClientTokenId", "UnrecognizedClientException", "SignatureDoesNotMatch", "AuthFailure":
			return AuthenticationFailed
		case "Throttling", "ThrottlingException", "RequestLimitExceeded", "TooManyRequestsException":
			return Throttled
		}
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			return TimedOut
		}
		return ConnectionFailed
	}
	return Unknown
}

// Message returns a safe explanation and recovery action on separate lines.
// It never includes provider text, resource names, URLs, or helper output.
func Message(err error) string {
	switch Classify(err) {
	case None:
		return ""
	case Interrupted:
		return "Scan interrupted.\nRun the scan again when ready."
	case TimedOut:
		return "Scan timed out.\nIncrease --timeout and retry."
	case PermissionDenied:
		return "Permission denied.\nCheck the documented read-only permissions for this source."
	case AuthenticationFailed:
		return "Authentication failed.\nRefresh or correct your credentials, then retry."
	case CredentialsExpired:
		return "Credentials expired.\nRefresh your credentials, then retry."
	case Throttled:
		return "Request throttled.\nWait and retry; for AWS scans, lower --concurrency."
	case ConnectionFailed:
		return "Connection failed.\nCheck network access and endpoint configuration, then retry."
	default:
		return "Request failed.\nCheck credentials, permissions, and connectivity, then retry."
	}
}

// AWSRegionDiscoveryMessage adds discovery-specific guidance only when relevant.
func AWSRegionDiscoveryMessage(err error) string {
	switch Classify(err) {
	case None:
		return ""
	case PermissionDenied:
		return "Region discovery was denied.\nAllow ec2:DescribeRegions, or select regions with --regions."
	case AuthenticationFailed:
		return "Authentication failed while discovering regions.\nRefresh or correct your AWS credentials, then retry."
	case CredentialsExpired:
		return "Credentials expired while discovering regions.\nRefresh your AWS credentials, then retry."
	case Unknown:
		return "Region discovery failed.\nCheck your AWS configuration and connectivity, then retry."
	default:
		return "Region discovery: " + Message(err)
	}
}
