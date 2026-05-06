package types

// ObjID represents a MOO object reference
// -1 = nothing, -2 = ambiguous, -3 = failed_match, 0+ = valid object
type ObjID int64

const (
	ObjNothing     ObjID = -1
	ObjAmbiguous   ObjID = -2
	ObjFailedMatch ObjID = -3
)
