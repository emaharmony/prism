package tool

import "testing"

func TestValidateGitRef(t *testing.T) {
	valid := []string{"", "main", "feature/x", "release-1.2.3", "v2.0", "a_b"}
	for _, r := range valid {
		if err := validateGitRef(r); err != nil {
			t.Errorf("validateGitRef(%q) = %v, want nil", r, err)
		}
	}

	invalid := []string{
		"--exec=evil",     // flag injection
		"-f",              // short flag
		"branch;rm -rf /", // shell metacharacters
		"a b",             // space
		"$(whoami)",       // command substitution
		"../escape",       // path traversal-ish
	}
	for _, r := range invalid {
		if err := validateGitRef(r); err == nil {
			t.Errorf("validateGitRef(%q) = nil, want error", r)
		}
	}
}

func TestValidateGitRemote(t *testing.T) {
	valid := []string{"", "origin", "upstream", "my-remote", "fork2"}
	for _, r := range valid {
		if err := validateGitRemote(r); err != nil {
			t.Errorf("validateGitRemote(%q) = %v, want nil", r, err)
		}
	}

	invalid := []string{
		"https://attacker.example/repo.git", // arbitrary URL (exfil)
		"git@github.com:evil/repo.git",      // scp-style URL
		"--receive-pack=evil",               // flag injection
		"host:path",                         // colon form
	}
	for _, r := range invalid {
		if err := validateGitRemote(r); err == nil {
			t.Errorf("validateGitRemote(%q) = nil, want error", r)
		}
	}
}
