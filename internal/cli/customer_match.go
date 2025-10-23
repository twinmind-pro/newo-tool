package cli

import (
	"strings"

	"github.com/twinmind/newo-tool/internal/customer"
)

func matchesCustomerToken(entry customer.Entry, remoteID string, token string) bool {
	token = strings.TrimSpace(token)
	if token == "" {
		return true
	}
	if strings.EqualFold(remoteID, token) {
		return true
	}
	if strings.EqualFold(entry.HintIDN, token) {
		return true
	}
	if entry.Alias != "" && strings.EqualFold(entry.Alias, token) {
		return true
	}
	return false
}
