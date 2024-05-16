package sat

// ResetSet represents a set of integers from 0 to N-1 where N is the capacity
// of the set.
type ResetSet struct {
	addedAt        []uint16
	addedTimestamp uint16
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
