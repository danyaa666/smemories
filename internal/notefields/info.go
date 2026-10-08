package notefields

// FieldInfo is the API shape of one field, for the form the public page renders.
type FieldInfo struct {
	ID        string `json:"id"`
	Kind      Kind   `json:"kind"`
	Label     Text   `json:"label"`
	Hint      Text   `json:"hint"`
	Required  bool   `json:"required"`
	MaxLength int    `json:"max_length"`
}

// Info returns the fields of refs in the order of refs.
func Info(refs []FieldRef) ([]FieldInfo, error) {
	fs, err := resolve(refs)
	if err != nil {
		return nil, err
	}
	out := make([]FieldInfo, len(fs))
	for i, f := range fs {
		out[i] = FieldInfo{f.ID, f.Kind, f.Label, f.Hint, refs[i].Required, f.MaxLength}
	}
	return out, nil
}
