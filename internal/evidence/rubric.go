package evidence

// SupportRubric returns a fresh map of the documentary-support-v1 anchors.
// The integer order is semantic support completeness, never confidence or truth.
func SupportRubric() map[string]string {
	return map[string]string{
		"0": "None: No valid, relevant supporting passage establishes a material part of the claim. A URL alone, an off-topic passage, or a mismatched subject does not qualify.",
		"1": "Partial or conditional: Evidence supports some of the claim, but at least one indispensable component or premise remains unestablished. An explicitly disclosed assumption remains an assumption.",
		"2": "Complete: Every material component and required premise has cited support at the claimed scope. Any derivation is explicit and checkable.",
	}
}
