package sat

type ResetSet struct {
	// Last timestamp at which a boolean variable was seen. This is effectively
	// used as a slice of boolean by the conflict analyze algorithm. Precisely,
	// a variable v is considered "seen" if addedAt[v] == addedTimestamp. All
	// the variables can efficiently be marked as "not seen" in constant time
	// by increasing the timestamp.
	addedAt        []uint64
	addedTimestamp uint64
}

// Contains returns true if v is in the set.
func (rs *ResetSet) Contains(v int) bool {
	return rs.addedAt[v] == rs.addedTimestamp
}

// Add adds v to the set.
func (rs *ResetSet) Add(v int) {
	rs.addedAt[v] = rs.addedTimestamp
}

// Clear removes all the elements in the set in constant time.
func (rs *ResetSet) Clear() {
	rs.addedTimestamp++
	if rs.addedTimestamp == 0 { // overflow
		rs.addedTimestamp = 1
		for i := range rs.addedAt {
			rs.addedAt[i] = 0
		}
	}
}

// Expand increases the capacity of the set.
func (rs *ResetSet) Expand() {
	rs.addedAt = append(rs.addedAt, 0)
}
