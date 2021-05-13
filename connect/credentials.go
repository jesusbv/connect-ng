package connect

import (
	"errors"
	"fmt"
	"os"
	"regexp"
)

const (
	defaulCredPath = "/etc/zypp/credentials.d/SCCcredentials"
)

var (
	ErrNoCredentialsFile = errors.New("Credentials file does not exist")
	userMatch            = regexp.MustCompile(`(?m)^\s*username\s*=\s*(\S+)\s*$`)
	passMatch            = regexp.MustCompile(`(?m)^\s*password\s*=\s*(\S+)\s*$`)
)

type Credentials struct {
	Username string
	Password string
}

func GetCredentials() (Credentials, error) {
	return LoadFile(defaulCredPath)
}

func LoadFile(path string) (Credentials, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Credentials{}, fmt.Errorf("%w; %s", ErrNoCredentialsFile, err)
		}
		return Credentials{}, err
	}
	uMatch := userMatch.FindStringSubmatch(string(content))
	pMatch := passMatch.FindStringSubmatch(string(content))
	if len(uMatch) != 2 || len(pMatch) != 2 {
		return Credentials{}, fmt.Errorf("Unable to parse credentials from %s", path)
	}
	return Credentials{Username: uMatch[1], Password: pMatch[1]}, nil
}
