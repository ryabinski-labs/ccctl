package session

import "sync"

// RingSize is the per-session scrollback (A-003).
const RingSize = 1 << 20

// Ring keeps the newest Cap bytes written.
type Ring struct {
	mu   sync.Mutex
	buf  []byte
	cap  int
	head int // next write index
	full bool
}

func NewRing(capacity int) *Ring { return &Ring{buf: make([]byte, capacity), cap: capacity} }

func (r *Ring) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := len(p)
	if n >= r.cap {
		copy(r.buf, p[n-r.cap:])
		r.head, r.full = 0, true
		return n, nil
	}
	k := copy(r.buf[r.head:], p)
	if k < n {
		copy(r.buf, p[k:])
		r.full = true
	}
	if r.head+n >= r.cap {
		r.full = true
	}
	r.head = (r.head + n) % r.cap
	return n, nil
}

// Snapshot returns a copy of the buffered bytes, oldest first.
func (r *Ring) Snapshot() []byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.full {
		return append([]byte(nil), r.buf[:r.head]...)
	}
	out := make([]byte, 0, r.cap)
	out = append(out, r.buf[r.head:]...)
	return append(out, r.buf[:r.head]...)
}
