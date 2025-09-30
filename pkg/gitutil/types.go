package gitutil

// Protocol represents the git protocol type.
type Protocol string

const (
	// ProtocolHTTPS represents HTTPS git protocol
	ProtocolHTTPS Protocol = "https"
	// ProtocolSSH represents SSH git protocol
	ProtocolSSH Protocol = "ssh"
	// ProtocolGit represents git protocol
	ProtocolGit Protocol = "git"
)

// RepoURL represents a parsed git repository URL with multiple access formats.
type RepoURL struct {
	// Host is the git hosting provider (e.g., github.com, gitlab.com)
	Host string

	// Owner is the repository owner or organization
	Owner string

	// Name is the repository name
	Name string

	// Protocol is the access protocol used in the original URL
	Protocol Protocol

	// CloneURL is the full clone URL in the original format
	CloneURL string

	// HTTPSURL is the HTTPS version of the clone URL
	HTTPSURL string

	// SSHURL is the SSH version of the clone URL
	SSHURL string
}
