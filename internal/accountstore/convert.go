package accountstore

import "interface-load-test/internal/accountpool"

// FilterEnabled keeps Enabled accounts in their original order.
func FilterEnabled(accounts []Account) []Account {
	filtered := make([]Account, 0, len(accounts))
	for _, acc := range accounts {
		if acc.Enabled {
			filtered = append(filtered, acc)
		}
	}
	return filtered
}

// ToPoolAccounts converts persisted accounts to execution accounts.
//
// Callers should FilterEnabled before converting.
func ToPoolAccounts(accounts []Account) []*accountpool.Account {
	poolAccounts := make([]*accountpool.Account, 0, len(accounts))
	for _, acc := range accounts {
		poolAccounts = append(poolAccounts, &accountpool.Account{
			ID:       acc.ID,
			Username: acc.Username,
			Password: acc.Password,
		})
	}
	return poolAccounts
}
