package gitutil

import (
	"testing"
)

func TestParseRepoURL(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantHost     string
		wantOwner    string
		wantName     string
		wantProtocol Protocol
		wantHTTPSURL string
		wantSSHURL   string
		wantErr      bool
	}{
		{
			name:         "shorthand user/repo",
			input:        "user/repo",
			wantHost:     "github.com",
			wantOwner:    "user",
			wantName:     "repo",
			wantProtocol: ProtocolHTTPS,
			wantHTTPSURL: "https://github.com/user/repo.git",
			wantSSHURL:   "git@github.com:user/repo.git",
		},
		{
			name:         "shorthand with .git suffix",
			input:        "user/repo.git",
			wantHost:     "github.com",
			wantOwner:    "user",
			wantName:     "repo",
			wantProtocol: ProtocolHTTPS,
			wantHTTPSURL: "https://github.com/user/repo.git",
			wantSSHURL:   "git@github.com:user/repo.git",
		},
		{
			name:         "host/user/repo format",
			input:        "github.com/user/repo",
			wantHost:     "github.com",
			wantOwner:    "user",
			wantName:     "repo",
			wantProtocol: ProtocolHTTPS,
			wantHTTPSURL: "https://github.com/user/repo.git",
			wantSSHURL:   "git@github.com:user/repo.git",
		},
		{
			name:         "gitlab host/user/repo",
			input:        "gitlab.com/user/repo",
			wantHost:     "gitlab.com",
			wantOwner:    "user",
			wantName:     "repo",
			wantProtocol: ProtocolHTTPS,
			wantHTTPSURL: "https://gitlab.com/user/repo.git",
			wantSSHURL:   "git@gitlab.com:user/repo.git",
		},
		{
			name:         "full HTTPS URL",
			input:        "https://github.com/user/repo.git",
			wantHost:     "github.com",
			wantOwner:    "user",
			wantName:     "repo",
			wantProtocol: ProtocolHTTPS,
			wantHTTPSURL: "https://github.com/user/repo.git",
			wantSSHURL:   "git@github.com:user/repo.git",
		},
		{
			name:         "full HTTPS URL without .git",
			input:        "https://github.com/user/repo",
			wantHost:     "github.com",
			wantOwner:    "user",
			wantName:     "repo",
			wantProtocol: ProtocolHTTPS,
			wantHTTPSURL: "https://github.com/user/repo.git",
			wantSSHURL:   "git@github.com:user/repo.git",
		},
		{
			name:         "SSH URL",
			input:        "git@github.com:user/repo.git",
			wantHost:     "github.com",
			wantOwner:    "user",
			wantName:     "repo",
			wantProtocol: ProtocolSSH,
			wantHTTPSURL: "https://github.com/user/repo.git",
			wantSSHURL:   "git@github.com:user/repo.git",
		},
		{
			name:         "SSH URL without .git",
			input:        "git@github.com:user/repo",
			wantHost:     "github.com",
			wantOwner:    "user",
			wantName:     "repo",
			wantProtocol: ProtocolSSH,
			wantHTTPSURL: "https://github.com/user/repo.git",
			wantSSHURL:   "git@github.com:user/repo.git",
		},
		{
			name:         "gitlab SSH URL",
			input:        "git@gitlab.com:user/repo.git",
			wantHost:     "gitlab.com",
			wantOwner:    "user",
			wantName:     "repo",
			wantProtocol: ProtocolSSH,
			wantHTTPSURL: "https://gitlab.com/user/repo.git",
			wantSSHURL:   "git@gitlab.com:user/repo.git",
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
		{
			name:    "invalid SSH format",
			input:   "git@github.com",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseRepoURL(tt.input)

			if (err != nil) != tt.wantErr {
				t.Errorf("ParseRepoURL() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				return
			}

			if got.Host != tt.wantHost {
				t.Errorf("ParseRepoURL() Host = %v, want %v", got.Host, tt.wantHost)
			}
			if got.Owner != tt.wantOwner {
				t.Errorf("ParseRepoURL() Owner = %v, want %v", got.Owner, tt.wantOwner)
			}
			if got.Name != tt.wantName {
				t.Errorf("ParseRepoURL() Name = %v, want %v", got.Name, tt.wantName)
			}
			if got.Protocol != tt.wantProtocol {
				t.Errorf("ParseRepoURL() Protocol = %v, want %v", got.Protocol, tt.wantProtocol)
			}
			if got.HTTPSURL != tt.wantHTTPSURL {
				t.Errorf("ParseRepoURL() HTTPSURL = %v, want %v", got.HTTPSURL, tt.wantHTTPSURL)
			}
			if got.SSHURL != tt.wantSSHURL {
				t.Errorf("ParseRepoURL() SSHURL = %v, want %v", got.SSHURL, tt.wantSSHURL)
			}
		})
	}
}

func TestBuildCloneURL(t *testing.T) {
	tests := []struct {
		name     string
		repo     string
		protocol Protocol
		want     string
		wantErr  bool
	}{
		{
			name:     "HTTPS protocol",
			repo:     "user/repo",
			protocol: ProtocolHTTPS,
			want:     "https://github.com/user/repo.git",
		},
		{
			name:     "SSH protocol",
			repo:     "user/repo",
			protocol: ProtocolSSH,
			want:     "git@github.com:user/repo.git",
		},
		{
			name:     "full URL with HTTPS protocol",
			repo:     "https://github.com/user/repo.git",
			protocol: ProtocolHTTPS,
			want:     "https://github.com/user/repo.git",
		},
		{
			name:     "full URL with SSH protocol",
			repo:     "git@github.com:user/repo.git",
			protocol: ProtocolSSH,
			want:     "git@github.com:user/repo.git",
		},
		{
			name:     "gitlab with HTTPS",
			repo:     "gitlab.com/user/repo",
			protocol: ProtocolHTTPS,
			want:     "https://gitlab.com/user/repo.git",
		},
		{
			name:    "empty repo",
			repo:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := BuildCloneURL(tt.repo, tt.protocol)

			if (err != nil) != tt.wantErr {
				t.Errorf("BuildCloneURL() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && got != tt.want {
				t.Errorf("BuildCloneURL() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNormalizeURL(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "HTTPS URL with .git",
			input: "https://github.com/user/repo.git",
			want:  "https://github.com/user/repo",
		},
		{
			name:  "HTTPS URL without .git",
			input: "https://github.com/user/repo",
			want:  "https://github.com/user/repo",
		},
		{
			name:  "SSH URL converts to HTTPS",
			input: "git@github.com:user/repo.git",
			want:  "https://github.com/user/repo",
		},
		{
			name:  "SSH URL without .git",
			input: "git@github.com:user/repo",
			want:  "https://github.com/user/repo",
		},
		{
			name:  "mixed case lowercased",
			input: "https://GitHub.com/User/Repo.git",
			want:  "https://github.com/user/repo",
		},
		{
			name:  "whitespace trimmed",
			input: "  https://github.com/user/repo.git  ",
			want:  "https://github.com/user/repo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeURL(tt.input)
			if got != tt.want {
				t.Errorf("NormalizeURL() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExtractRepoName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "user/repo format",
			input: "user/repo",
			want:  "repo",
		},
		{
			name:  "with .git suffix",
			input: "user/repo.git",
			want:  "repo",
		},
		{
			name:  "full HTTPS URL",
			input: "https://github.com/user/repo.git",
			want:  "repo",
		},
		{
			name:  "SSH URL",
			input: "git@github.com:user/repo.git",
			want:  "repo",
		},
		{
			name:  "host/user/repo format",
			input: "github.com/user/repo",
			want:  "repo",
		},
		{
			name:  "just repo name",
			input: "repo",
			want:  "repo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractRepoName(tt.input)
			if got != tt.want {
				t.Errorf("ExtractRepoName() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExtractOwnerAndRepo(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantOwner string
		wantName  string
		wantErr   bool
	}{
		{
			name:      "user/repo format",
			input:     "user/repo",
			wantOwner: "user",
			wantName:  "repo",
		},
		{
			name:      "full HTTPS URL",
			input:     "https://github.com/user/repo.git",
			wantOwner: "user",
			wantName:  "repo",
		},
		{
			name:      "SSH URL",
			input:     "git@github.com:user/repo.git",
			wantOwner: "user",
			wantName:  "repo",
		},
		{
			name:      "host/user/repo format",
			input:     "github.com/user/repo",
			wantOwner: "user",
			wantName:  "repo",
		},
		{
			name:    "just repo name (no owner)",
			input:   "repo",
			wantErr: true, // ParseRepoURL will assign empty owner, which triggers error in ExtractOwnerAndRepo
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOwner, gotName, err := ExtractOwnerAndRepo(tt.input)

			if (err != nil) != tt.wantErr {
				t.Errorf("ExtractOwnerAndRepo() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				return
			}

			if gotOwner != tt.wantOwner {
				t.Errorf("ExtractOwnerAndRepo() owner = %v, want %v", gotOwner, tt.wantOwner)
			}
			if gotName != tt.wantName {
				t.Errorf("ExtractOwnerAndRepo() name = %v, want %v", gotName, tt.wantName)
			}
		})
	}
}
