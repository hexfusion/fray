package oci

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseChallenge(t *testing.T) {
	tests := []struct {
		name      string
		header    string
		wantRealm string
		wantSvc   string
		wantScope string
	}{
		{
			name:      "docker hub challenge",
			header:    `Bearer realm="https://auth.docker.io/token",service="registry.docker.io",scope="repository:library/nginx:pull"`,
			wantRealm: "https://auth.docker.io/token",
			wantSvc:   "registry.docker.io",
			wantScope: "repository:library/nginx:pull",
		},
		{
			name:      "quay challenge",
			header:    `Bearer realm="https://quay.io/v2/auth",service="quay.io"`,
			wantRealm: "https://quay.io/v2/auth",
			wantSvc:   "quay.io",
			wantScope: "",
		},
		{
			name:      "empty header",
			header:    "",
			wantRealm: "",
			wantSvc:   "",
			wantScope: "",
		},
		{
			name:      "realm only",
			header:    `Bearer realm="https://example.com/auth"`,
			wantRealm: "https://example.com/auth",
			wantSvc:   "",
			wantScope: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require := require.New(t)

			ch := parseChallenge(tt.header)

			if tt.header == "" {
				require.Nil(ch)
				return
			}

			require.NotNil(ch)
			require.Equal(tt.wantRealm, ch.realm)
			require.Equal(tt.wantSvc, ch.service)
			require.Equal(tt.wantScope, ch.scope)
		})
	}
}

func TestDecodeAuth(t *testing.T) {
	tests := []struct {
		name         string
		encoded      string
		wantUsername string
		wantPassword string
		wantErr      bool
	}{
		{
			name:         "valid credentials",
			encoded:      "dXNlcjpwYXNz", // base64("user:pass")
			wantUsername: "user",
			wantPassword: "pass",
			wantErr:      false,
		},
		{
			name:         "password with colon",
			encoded:      "dXNlcjpwYXNzOndvcmQ=", // base64("user:pass:word")
			wantUsername: "user",
			wantPassword: "pass:word",
			wantErr:      false,
		},
		{
			name:         "empty password",
			encoded:      "dXNlcjo=", // base64("user:")
			wantUsername: "user",
			wantPassword: "",
			wantErr:      false,
		},
		{
			name:    "invalid base64",
			encoded: "not-valid-base64!!!",
			wantErr: true,
		},
		{
			name:    "no colon separator",
			encoded: "dXNlcm5hbWU=", // base64("username")
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require := require.New(t)

			username, password, err := decodeAuth(tt.encoded)

			if tt.wantErr {
				require.Error(err)
			} else {
				require.NoError(err)
				require.Equal(tt.wantUsername, username)
				require.Equal(tt.wantPassword, password)
			}
		})
	}
}
