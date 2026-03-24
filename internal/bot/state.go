package bot

import (
	"sync"
	"time"

	"github.com/igorkon/youtube-downloader-bot/pkg/types"
)

// pendingExpiry is the maximum age of a pending download before it expires.
const pendingExpiry = 5 * time.Minute

// PendingDownload holds the state for a user who has been shown format options
// but has not yet selected one.
type PendingDownload struct {
	UserID    int64
	URL       string
	Formats   []types.Format
	CreatedAt time.Time
}

// state manages pending download selections in memory.
type state struct {
	mu      sync.RWMutex
	pending map[int64]*PendingDownload
}

// newState creates a new empty state manager.
func newState() *state {
	return &state{
		pending: make(map[int64]*PendingDownload),
	}
}

// set stores a pending download for the given user, replacing any existing one.
func (s *state) set(userID int64, pd *PendingDownload) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending[userID] = pd
}

// get retrieves a pending download for the user. Returns nil if not found or expired.
// Expired entries are automatically removed.
func (s *state) get(userID int64) *PendingDownload {
	s.mu.Lock()
	defer s.mu.Unlock()

	pd, ok := s.pending[userID]
	if !ok {
		return nil
	}

	if time.Since(pd.CreatedAt) > pendingExpiry {
		delete(s.pending, userID)
		return nil
	}

	return pd
}

// delete removes a pending download for the user.
func (s *state) delete(userID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.pending, userID)
}

// isExpired checks whether a PendingDownload has expired without removing it.
func isExpired(pd *PendingDownload) bool {
	return time.Since(pd.CreatedAt) > pendingExpiry
}
