package cel

import (
	"encoding/base64"
	"net"
	"net/mail"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

// ValidateEmail validates an RFC 5322 email address.
func ValidateEmail(s string) error {
	if s == "" {
		return &FormatError{Format: "email", Value: s, Reason: "empty"}
	}
	_, err := mail.ParseAddress(s)
	if err != nil {
		return &FormatError{Format: "email", Value: s, Reason: err.Error()}
	}
	return nil
}

// ValidateURI validates an RFC 3986 absolute URI.
func ValidateURI(s string) error {
	if s == "" {
		return &FormatError{Format: "uri", Value: s, Reason: "empty"}
	}
	u, err := url.Parse(s)
	if err != nil {
		return &FormatError{Format: "uri", Value: s, Reason: err.Error()}
	}
	if u.Scheme == "" {
		return &FormatError{Format: "uri", Value: s, Reason: "missing scheme"}
	}
	return nil
}

// ValidateURIRef validates an RFC 3986 URI reference (absolute or relative).
func ValidateURIRef(s string) error {
	if s == "" {
		return &FormatError{Format: "uri_ref", Value: s, Reason: "empty"}
	}
	_, err := url.Parse(s)
	if err != nil {
		return &FormatError{Format: "uri_ref", Value: s, Reason: err.Error()}
	}
	return nil
}

// ValidateHostname validates an RFC 1123 hostname.
func ValidateHostname(s string) error {
	if s == "" {
		return &FormatError{Format: "hostname", Value: s, Reason: "empty"}
	}
	if len(s) > 253 {
		return &FormatError{Format: "hostname", Value: s, Reason: "exceeds 253 characters"}
	}
	if !hostnameRegex.MatchString(s) {
		return &FormatError{Format: "hostname", Value: s, Reason: "invalid format"}
	}
	return nil
}

// ValidateIPv4 validates an IPv4 address.
func ValidateIPv4(s string) error {
	if s == "" {
		return &FormatError{Format: "ipv4", Value: s, Reason: "empty"}
	}
	ip := net.ParseIP(s)
	if ip == nil {
		return &FormatError{Format: "ipv4", Value: s, Reason: "invalid IP"}
	}
	if ip.To4() == nil {
		return &FormatError{Format: "ipv4", Value: s, Reason: "not an IPv4 address"}
	}
	return nil
}

// ValidateIPv6 validates an IPv6 address.
func ValidateIPv6(s string) error {
	if s == "" {
		return &FormatError{Format: "ipv6", Value: s, Reason: "empty"}
	}
	ip := net.ParseIP(s)
	if ip == nil {
		return &FormatError{Format: "ipv6", Value: s, Reason: "invalid IP"}
	}
	if ip.To4() != nil {
		return &FormatError{Format: "ipv6", Value: s, Reason: "not an IPv6 address"}
	}
	return nil
}

// ValidateIP validates an IPv4 or IPv6 address.
func ValidateIP(s string) error {
	if s == "" {
		return &FormatError{Format: "ip", Value: s, Reason: "empty"}
	}
	if net.ParseIP(s) == nil {
		return &FormatError{Format: "ip", Value: s, Reason: "invalid IP address"}
	}
	return nil
}

// ValidateUUID validates an RFC 4122 UUID.
func ValidateUUID(s string) error {
	if s == "" {
		return &FormatError{Format: "uuid", Value: s, Reason: "empty"}
	}
	if !uuidRegex.MatchString(s) {
		return &FormatError{Format: "uuid", Value: s, Reason: "invalid UUID format"}
	}
	return nil
}

// ValidateUUIDv4 validates a UUID version 4.
func ValidateUUIDv4(s string) error {
	if err := ValidateUUID(s); err != nil {
		return err
	}
	// Version 4 has 4 in position 14 and 8,9,a,b in position 19
	if len(s) != 36 || s[14] != '4' {
		return &FormatError{Format: "uuid_v4", Value: s, Reason: "not version 4"}
	}
	c := s[19]
	if c != '8' && c != '9' && c != 'a' && c != 'b' && c != 'A' && c != 'B' {
		return &FormatError{Format: "uuid_v4", Value: s, Reason: "invalid variant"}
	}
	return nil
}

// ValidateDNSLabel validates a Kubernetes-style DNS label.
// Max 63 chars, lowercase alphanumeric, may contain hyphens (not at start/end).
func ValidateDNSLabel(s string) error {
	if s == "" {
		return &FormatError{Format: "dns_label", Value: s, Reason: "empty"}
	}
	if len(s) > 63 {
		return &FormatError{Format: "dns_label", Value: s, Reason: "exceeds 63 characters"}
	}
	if !dnsLabelRegex.MatchString(s) {
		return &FormatError{Format: "dns_label", Value: s, Reason: "must be lowercase alphanumeric with optional hyphens, not starting or ending with hyphen"}
	}
	return nil
}

// ValidateDNSSubdomain validates a Kubernetes-style DNS subdomain.
// Max 253 chars, lowercase alphanumeric, may contain hyphens and dots.
func ValidateDNSSubdomain(s string) error {
	if s == "" {
		return &FormatError{Format: "dns_subdomain", Value: s, Reason: "empty"}
	}
	if len(s) > 253 {
		return &FormatError{Format: "dns_subdomain", Value: s, Reason: "exceeds 253 characters"}
	}
	parts := strings.Split(s, ".")
	for _, part := range parts {
		if err := ValidateDNSLabel(part); err != nil {
			return &FormatError{Format: "dns_subdomain", Value: s, Reason: "invalid label: " + part}
		}
	}
	return nil
}

// ValidateQualifiedName validates a Kubernetes qualified name (prefix/name).
func ValidateQualifiedName(s string) error {
	if s == "" {
		return &FormatError{Format: "qualified_name", Value: s, Reason: "empty"}
	}
	parts := strings.Split(s, "/")
	switch len(parts) {
	case 1:
		return ValidateDNSLabel(s)
	case 2:
		if err := ValidateDNSSubdomain(parts[0]); err != nil {
			return &FormatError{Format: "qualified_name", Value: s, Reason: "invalid prefix"}
		}
		if err := ValidateDNSLabel(parts[1]); err != nil {
			return &FormatError{Format: "qualified_name", Value: s, Reason: "invalid name"}
		}
		return nil
	default:
		return &FormatError{Format: "qualified_name", Value: s, Reason: "too many slashes"}
	}
}

// ValidateImageRef validates a container image reference.
func ValidateImageRef(s string) error {
	if s == "" {
		return &FormatError{Format: "image_ref", Value: s, Reason: "empty"}
	}
	if !imageRefRegex.MatchString(s) {
		return &FormatError{Format: "image_ref", Value: s, Reason: "invalid image reference"}
	}
	return nil
}

// ValidateImageTag validates a container image tag.
func ValidateImageTag(s string) error {
	if s == "" {
		return &FormatError{Format: "image_tag", Value: s, Reason: "empty"}
	}
	if len(s) > 128 {
		return &FormatError{Format: "image_tag", Value: s, Reason: "exceeds 128 characters"}
	}
	if !imageTagRegex.MatchString(s) {
		return &FormatError{Format: "image_tag", Value: s, Reason: "invalid characters"}
	}
	return nil
}

// ValidateImageDigest validates a container image digest.
func ValidateImageDigest(s string) error {
	if s == "" {
		return &FormatError{Format: "image_digest", Value: s, Reason: "empty"}
	}
	if !imageDigestRegex.MatchString(s) {
		return &FormatError{Format: "image_digest", Value: s, Reason: "must be algorithm:hex format (e.g., sha256:...)"}
	}
	return nil
}

// ValidateDate validates an RFC 3339 date (YYYY-MM-DD).
func ValidateDate(s string) error {
	if s == "" {
		return &FormatError{Format: "date", Value: s, Reason: "empty"}
	}
	_, err := time.Parse("2006-01-02", s)
	if err != nil {
		return &FormatError{Format: "date", Value: s, Reason: "must be YYYY-MM-DD"}
	}
	return nil
}

// ValidateDatetime validates an RFC 3339 datetime.
func ValidateDatetime(s string) error {
	if s == "" {
		return &FormatError{Format: "datetime", Value: s, Reason: "empty"}
	}
	_, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return &FormatError{Format: "datetime", Value: s, Reason: "must be RFC 3339 format"}
	}
	return nil
}

// ValidateDuration validates a Go-style duration string.
func ValidateDuration(s string) error {
	if s == "" {
		return &FormatError{Format: "duration", Value: s, Reason: "empty"}
	}
	_, err := time.ParseDuration(s)
	if err != nil {
		return &FormatError{Format: "duration", Value: s, Reason: err.Error()}
	}
	return nil
}

// ValidateSemver validates a semantic version string.
func ValidateSemver(s string) error {
	if s == "" {
		return &FormatError{Format: "semver", Value: s, Reason: "empty"}
	}
	if !semverRegex.MatchString(s) {
		return &FormatError{Format: "semver", Value: s, Reason: "invalid semver format"}
	}
	return nil
}

// ValidateBase64 validates a base64-encoded string.
func ValidateBase64(s string) error {
	if s == "" {
		return &FormatError{Format: "base64", Value: s, Reason: "empty"}
	}
	_, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return &FormatError{Format: "base64", Value: s, Reason: "invalid base64 encoding"}
	}
	return nil
}

// ValidatePEM validates a PEM-encoded block.
func ValidatePEM(s string) error {
	if s == "" {
		return &FormatError{Format: "pem", Value: s, Reason: "empty"}
	}
	if !strings.Contains(s, "-----BEGIN ") || !strings.Contains(s, "-----END ") {
		return &FormatError{Format: "pem", Value: truncate(s, 50), Reason: "missing PEM headers"}
	}
	return nil
}

// FormatError represents a format validation failure.
type FormatError struct {
	Format string
	Value  string
	Reason string
}

func (e *FormatError) Error() string {
	v := truncate(e.Value, 30)
	return "invalid " + e.Format + ": " + v + " (" + e.Reason + ")"
}

func truncate(s string, maxLen int) string {
	if utf8.RuneCountInString(s) <= maxLen {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxLen-3]) + "..."
}

var (
	hostnameRegex    = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$`)
	uuidRegex        = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	dnsLabelRegex    = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)
	imageRefRegex    = regexp.MustCompile(`^[a-z0-9]+([._-][a-z0-9]+)*(/[a-z0-9]+([._-][a-z0-9]+)*)*(:[\w][\w.-]{0,127})?(@[a-z0-9]+:[a-f0-9]+)?$`)
	imageTagRegex    = regexp.MustCompile(`^[\w][\w.-]{0,127}$`)
	imageDigestRegex = regexp.MustCompile(`^[a-z0-9]+:[a-f0-9]+$`)
	semverRegex      = regexp.MustCompile(`^v?(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(-[a-zA-Z0-9]+(\.[a-zA-Z0-9]+)*)?(\+[a-zA-Z0-9]+(\.[a-zA-Z0-9]+)*)?$`)
)
