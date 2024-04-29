package sat

// LBool represents a lifted boolean. That is, a boolean that can either be
// True, False, or Unknown.
type LBool int8

const (
	Unknown LBool = 0
	True    LBool = 1
	False   LBool = -1
)

// Opposite returns the opposite of the lifted boolean as follows:
//
//	True -> False
//	False -> True
//	Unknown -> Unknown
func (l LBool) Opposite() LBool {
	return -l
}

// Lift returns a LBool corresponding to the given bool.
func Lift(b bool) LBool {
	if b {
		return True
	} else {
		return False
	}
}

func (l LBool) String() string {
	switch l {
	case True:
		return "true"
	case False:
		return "false"
	default:
		return "unknown"
	}
}
