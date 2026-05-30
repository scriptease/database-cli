package daemon

import (
	"fmt"
	"os/exec"
	"strings"
)

func lookupKeychainPassword(service string) (string, error) {
	output, err := exec.Command("security", "find-generic-password", "-a", "jdbc-cli", "-s", service, "-w").CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		return "", fmt.Errorf("keychain lookup failed for %q: %s", service, message)
	}
	return strings.TrimSpace(string(output)), nil
}
