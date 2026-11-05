package evidence

// Citation retains a checked link from a scoped claim to exact snapshot bytes.
type Citation struct {
	ClaimID      string `json:"claim_id"`
	PassageID    string `json:"passage_id"`
	SourceID     string `json:"source_id"`
	SourceSHA256 string `json:"source_sha256"`
	Excerpt      string `json:"excerpt"`
	StartByte    int    `json:"start_byte"`
	EndByte      int    `json:"end_byte"`
	Scope        Scope  `json:"scope"`
	Relation     string `json:"relation,omitempty"`
	Accepted     bool   `json:"accepted"`
}

// SourceRecord retains supplied provenance; independent origins are unverified.
type SourceRecord struct {
	EvidenceKind string   `json:"evidence_kind"`
	Method       string   `json:"method,omitempty"`
	Authority    string   `json:"authority,omitempty"`
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	URI          string   `json:"uri,omitempty"`
	SHA256       string   `json:"sha256"`
	Origin       string   `json:"origin"`
	OriginGroup  string   `json:"origin_group"`
	Independence string   `json:"independence"`
	DerivedFrom  []string `json:"derived_from,omitempty"`
	CapturedAt   string   `json:"captured_at,omitempty"`
	PublishedAt  string   `json:"published_at,omitempty"`
	ValidFrom    string   `json:"valid_from,omitempty"`
	ValidUntil   string   `json:"valid_until,omitempty"`
	Scope        Scope    `json:"scope"`
}

// ModelIdentity identifies each backend/model combination actually observed.
type ModelIdentity struct {
	Provider       string `json:"provider,omitempty"`
	RequestedModel string `json:"requested_model,omitempty"`
	ResolvedModel  string `json:"resolved_model,omitempty"`
}

func (s *session) audit() {
	s.r.Citations = []Citation{}
	s.r.Sources = []SourceRecord{}
	s.r.Models = []ModelIdentity{}
	for i := range s.p.Sources {
		src := &s.p.Sources[i]
		s.r.Sources = append(s.r.Sources, sourceRecord(src, s.origin[src.ID]))
	}
	for i := range s.r.Claims {
		c := &s.r.Claims[i]
		for j := range c.Passages {
			p := &c.Passages[j]
			input := s.passage[p.PassageID]
			src := s.source[input.SourceID]
			s.r.Citations = append(s.r.Citations, Citation{ClaimID: c.ClaimID, PassageID: p.PassageID, SourceID: src.ID, SourceSHA256: src.SHA256, Excerpt: input.Excerpt, StartByte: input.StartByte, EndByte: input.EndByte, Scope: src.Scope, Relation: p.Relation, Accepted: p.Applicable && p.Relation == Supports && p.Answer != nil && p.Answer.Validity == ValiditySuccess})
			s.recordModel(p.Answer)
		}
		s.recordModel(c.Answer)
		s.recordModel(c.DirectnessAnswer)
	}
	s.recordModel(s.r.Choice)
	if len(s.r.Models) > 1 {
		s.r.Calibration = "advisory_model_identity_changed"
		s.r.Issues = append(s.r.Issues, Issue{Severity: "warning", Code: "model_identity_changed", Path: "models", Message: "Provider or resolved model identity varied; calibration does not transfer across identities"})
	}
}

func (s *session) recordModel(a *Answer) {
	if a == nil {
		return
	}
	id := ModelIdentity{Provider: a.Provider, RequestedModel: a.RequestedModel, ResolvedModel: a.ResolvedModel}
	if id.Provider == "" && id.RequestedModel == "" && id.ResolvedModel == "" {
		return
	}
	for _, existing := range s.r.Models {
		if existing == id {
			return
		}
	}
	s.r.Models = append(s.r.Models, id)
}

func sourceRecord(src *Source, group string) SourceRecord {
	independence := "declared_unverified"
	if src.Origin == "" || src.Origin == Unknown {
		independence = Unknown
	}
	return SourceRecord{EvidenceKind: src.EvidenceKind, Method: src.Method, Authority: src.Authority, ID: src.ID, Title: src.Title, URI: src.URI, SHA256: src.SHA256, Origin: src.Origin, OriginGroup: group, Independence: independence, DerivedFrom: src.DerivedFrom, CapturedAt: src.CapturedAt, PublishedAt: src.PublishedAt, ValidFrom: src.ValidFrom, ValidUntil: src.ValidUntil, Scope: src.Scope}
}
