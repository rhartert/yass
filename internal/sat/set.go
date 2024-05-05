package sat

type ResetSet struct {
	// Last timestamp at which a boolean variable was seen. This is effectively
	// used as a slice of boolean by the conflict analyze algorithm. Precisely,
	// a variable v is considered "seen" if seenAt[v] == seemTimestamp. All the
	// variables can efficiently be marked as "not seen" in constant time by
	// increasing the timestamp.
	seenAt        []uint64
	seenTimestamp uint64
}

// Contains returns true if v has been marked as seen since the last time
// resetSeen was called.
func (rs *ResetSet) Contains(i int) bool {
	return rs.seenAt[i] == rs.seenTimestamp
}

// Add marks v as seen. It will remain seen until resetSeen is called.
func (rs *ResetSet) Add(i int) {
	rs.seenAt[i] = rs.seenTimestamp
}

func (rs *ResetSet) Remove(i int) {
	rs.seenAt[i] = rs.seenTimestamp - 1
}

// Reset marks all variables as "not seen" in amortized constant time.
func (rs *ResetSet) Reset() {
	rs.seenTimestamp++
	if rs.seenTimestamp == 0 { // overflow
		rs.seenTimestamp = 1
		for i := range rs.seenAt {
			rs.seenAt[i] = 0
		}
	}
}

// Expand increases the size of the set.
func (rs *ResetSet) Expand() {
	rs.seenAt = append(rs.seenAt, 0)
}
