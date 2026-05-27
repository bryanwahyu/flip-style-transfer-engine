package ledger

import "errors"

// ErrDoubleEntryViolation is the only error the ledger package owns.
// All other financial errors (insufficient funds, currency mismatch) belong
// to the money package where they are defined.
var ErrDoubleEntryViolation = errors.New("ledger entries do not sum to zero")
