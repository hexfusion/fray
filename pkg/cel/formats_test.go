package cel

import (
	"errors"
	"testing"
)

func TestValidateEmail(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid simple", "user@example.com", false},
		{"valid with plus", "user+tag@example.com", false},
		{"valid with dots", "first.last@example.com", false},
		{"valid subdomain", "user@mail.example.com", false},
		{"empty", "", true},
		{"missing at", "userexample.com", true},
		{"missing domain", "user@", true},
		{"missing local", "@example.com", true},
		{"spaces", "user @example.com", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateEmail(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateEmail(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if err != nil {
				var fe *FormatError
				if !errors.As(err, &fe) {
					t.Errorf("error should be *FormatError, got %T", err)
				}
			}
		})
	}
}

func TestValidateURI(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid http", "http://example.com", false},
		{"valid https", "https://example.com/path", false},
		{"valid with port", "https://example.com:8080", false},
		{"valid with query", "https://example.com?foo=bar", false},
		{"valid ftp", "ftp://files.example.com", false},
		{"empty", "", true},
		{"no scheme", "example.com", true},
		{"relative path", "/path/to/resource", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateURI(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateURI(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestValidateURIRef(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid absolute", "https://example.com", false},
		{"valid relative", "/path/to/resource", false},
		{"valid fragment", "#section", false},
		{"valid query only", "?foo=bar", false},
		{"empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateURIRef(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateURIRef(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestValidateHostname(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid simple", "example", false},
		{"valid domain", "example.com", false},
		{"valid subdomain", "sub.example.com", false},
		{"valid with hyphen", "my-host.example.com", false},
		{"valid with numbers", "host1.example.com", false},
		{"empty", "", true},
		{"starts with hyphen", "-example.com", true},
		{"ends with hyphen", "example-.com", true},
		{"too long", string(make([]byte, 254)), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateHostname(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateHostname(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestValidateIPv4(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid", "192.168.1.1", false},
		{"valid loopback", "127.0.0.1", false},
		{"valid zeros", "0.0.0.0", false},
		{"valid broadcast", "255.255.255.255", false},
		{"empty", "", true},
		{"ipv6", "::1", true},
		{"invalid octet", "256.1.1.1", true},
		{"too few octets", "192.168.1", true},
		{"letters", "abc.def.ghi.jkl", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateIPv4(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateIPv4(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestValidateIPv6(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid loopback", "::1", false},
		{"valid full", "2001:0db8:85a3:0000:0000:8a2e:0370:7334", false},
		{"valid compressed", "2001:db8::1", false},
		{"empty", "", true},
		{"ipv4", "192.168.1.1", true},
		{"invalid chars", "2001:xyz::1", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateIPv6(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateIPv6(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestValidateIP(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid ipv4", "192.168.1.1", false},
		{"valid ipv6", "::1", false},
		{"valid ipv6 full", "2001:db8::1", false},
		{"empty", "", true},
		{"invalid", "not-an-ip", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateIP(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateIP(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestValidateUUID(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid lowercase", "550e8400-e29b-41d4-a716-446655440000", false},
		{"valid uppercase", "550E8400-E29B-41D4-A716-446655440000", false},
		{"valid mixed", "550e8400-E29B-41d4-A716-446655440000", false},
		{"empty", "", true},
		{"no hyphens", "550e8400e29b41d4a716446655440000", true},
		{"wrong format", "550e8400-e29b-41d4-a716-4466554400", true},
		{"invalid chars", "550e8400-e29b-41d4-a716-44665544000g", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateUUID(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateUUID(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestValidateUUIDv4(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid v4", "550e8400-e29b-41d4-a716-446655440000", false},
		{"valid v4 variant 8", "f47ac10b-58cc-4372-8567-0e02b2c3d479", false},
		{"valid v4 variant 9", "f47ac10b-58cc-4372-9567-0e02b2c3d479", false},
		{"valid v4 variant a", "f47ac10b-58cc-4372-a567-0e02b2c3d479", false},
		{"valid v4 variant b", "f47ac10b-58cc-4372-b567-0e02b2c3d479", false},
		{"empty", "", true},
		{"v1 uuid", "550e8400-e29b-11d4-a716-446655440000", true},
		{"wrong variant", "550e8400-e29b-41d4-c716-446655440000", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateUUIDv4(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateUUIDv4(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestValidateDNSLabel(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid simple", "myapp", false},
		{"valid with hyphen", "my-app", false},
		{"valid with numbers", "app123", false},
		{"valid single char", "a", false},
		{"valid max length", "a23456789012345678901234567890123456789012345678901234567890123", false},
		{"empty", "", true},
		{"starts with hyphen", "-myapp", true},
		{"ends with hyphen", "myapp-", true},
		{"uppercase", "MyApp", true},
		{"too long", "a234567890123456789012345678901234567890123456789012345678901234", true},
		{"contains dot", "my.app", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDNSLabel(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateDNSLabel(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestValidateDNSSubdomain(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid simple", "myapp", false},
		{"valid subdomain", "my.app.example", false},
		{"valid with hyphens", "my-app.my-domain", false},
		{"empty", "", true},
		{"invalid label", "my.-app", true},
		{"too long", string(make([]byte, 254)), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDNSSubdomain(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateDNSSubdomain(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestValidateQualifiedName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid simple", "myname", false},
		{"valid qualified", "example.com/myname", false},
		{"valid with subdomain", "sub.example.com/myname", false},
		{"empty", "", true},
		{"too many slashes", "example.com/foo/bar", true},
		{"invalid prefix", "EXAMPLE.com/myname", true},
		{"invalid name", "example.com/MYNAME", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateQualifiedName(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateQualifiedName(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestValidateImageRef(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid simple", "nginx", false},
		{"valid with tag", "nginx:latest", false},
		{"valid with registry", "registry.example.com/nginx", false},
		{"valid with digest", "nginx@sha256:abc123", false},
		{"valid full", "registry.example.com/library/nginx:v1.0", false},
		{"empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateImageRef(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateImageRef(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestValidateImageTag(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid latest", "latest", false},
		{"valid version", "v1.0.0", false},
		{"valid with dot", "1.0", false},
		{"valid with hyphen", "v1-beta", false},
		{"empty", "", true},
		{"too long", string(make([]byte, 129)), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateImageTag(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateImageTag(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestValidateImageDigest(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid sha256", "sha256:abc123def456", false},
		{"valid sha512", "sha512:abc123def456789", false},
		{"empty", "", true},
		{"no algorithm", "abc123def456", true},
		{"uppercase hex", "sha256:ABC123", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateImageDigest(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateImageDigest(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestValidateDate(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid", "2024-01-15", false},
		{"valid leap year", "2024-02-29", false},
		{"empty", "", true},
		{"invalid format", "01-15-2024", true},
		{"invalid month", "2024-13-01", true},
		{"invalid day", "2024-01-32", true},
		{"with time", "2024-01-15T10:00:00Z", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDate(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateDate(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestValidateDatetime(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid utc", "2024-01-15T10:30:00Z", false},
		{"valid with offset", "2024-01-15T10:30:00+05:00", false},
		{"valid negative offset", "2024-01-15T10:30:00-08:00", false},
		{"empty", "", true},
		{"date only", "2024-01-15", true},
		{"no timezone", "2024-01-15T10:30:00", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDatetime(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateDatetime(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestValidateDuration(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid seconds", "30s", false},
		{"valid minutes", "5m", false},
		{"valid hours", "2h", false},
		{"valid combined", "1h30m", false},
		{"valid with ms", "100ms", false},
		{"valid negative", "-5m", false},
		{"empty", "", true},
		{"invalid unit", "5x", true},
		{"no unit", "100", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDuration(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateDuration(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestValidateSemver(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid basic", "1.0.0", false},
		{"valid with v", "v1.0.0", false},
		{"valid prerelease", "1.0.0-alpha", false},
		{"valid prerelease dot", "1.0.0-alpha.1", false},
		{"valid build", "1.0.0+build", false},
		{"valid full", "1.0.0-beta.1+build.123", false},
		{"empty", "", true},
		{"missing patch", "1.0", true},
		{"leading zero major", "01.0.0", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSemver(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateSemver(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestValidateBase64(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid", "SGVsbG8gV29ybGQ=", false},
		{"valid with padding", "SGVsbG9Xb3JsZA==", false},
		{"empty", "", true},
		{"invalid chars", "SGVsbG8!", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateBase64(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateBase64(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestValidatePEM(t *testing.T) {
	validPEM := `-----BEGIN CERTIFICATE-----
MIIBkTCB+wIJAKHBfpA5Q5T0MA0GCSqGSIb3DQEBCwUAMBExDzANBgNVBAMMBnRl
-----END CERTIFICATE-----`

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid", validPEM, false},
		{"empty", "", true},
		{"no begin", "-----END CERTIFICATE-----", true},
		{"no end", "-----BEGIN CERTIFICATE-----", true},
		{"plain text", "not a pem block", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePEM(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePEM() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestFormatError(t *testing.T) {
	tests := []struct {
		name     string
		err      *FormatError
		contains []string
	}{
		{
			"basic error",
			&FormatError{Format: "email", Value: "bad", Reason: "invalid"},
			[]string{"email", "bad", "invalid"},
		},
		{
			"truncated value",
			&FormatError{Format: "uri", Value: "this is a very long value that should be truncated", Reason: "too long"},
			[]string{"uri", "...", "too long"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := tt.err.Error()
			for _, s := range tt.contains {
				if !contains(msg, s) {
					t.Errorf("error message %q should contain %q", msg, s)
				}
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr))
}

func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
