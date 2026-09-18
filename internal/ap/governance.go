package ap

import "github.com/patchwork-toolkit/patchwork/internal/model"

// GovernanceContext returns the JSON-LD context with the gv: namespace.
func GovernanceContext() []interface{} {
	return []interface{}{
		"https://www.w3.org/ns/activitystreams",
		map[string]string{
			"gv": "https://" + GetDomain() + "/ns/governance#",
		},
	}
}

// Attributing a governance object to the person who wrote it.
//
// A proposal's author and a charter's editor are necessarily members of the
// patch — you cannot write either from outside it — so an `attributedTo`
// naming them asserts that membership to every remote reader. Where the
// membership is switched out of sight (docs/adr/006) that assertion is the
// one fact the switch took down, and ADR 006 is violated by an inference as
// surely as by a field.
//
// The builders below therefore refuse to guess. `attributeAuthor` is the
// caller's answer, and a caller with no answer must pass false: an
// unattributed object is a patch publishing its own text, which is what the
// object is for, while a wrongly attributed one cannot be recalled off the
// wire. Suppressing the whole object was considered and rejected here — the
// text is the patch's public governance record and withholding it would put
// a member's private switch in charge of what the patch may publish. A vote
// is the opposite case and is suppressed whole; see VoteToActivity.

// ProposalToObject converts a Proposal to an AP gv:Proposal object.
func ProposalToObject(p model.Proposal, domain string, attributeAuthor bool) map[string]interface{} {
	obj := map[string]interface{}{
		"@context":         GovernanceContext(),
		"type":             "gv:Proposal",
		"id":               APID(domain, "proposals", p.ID),
		"name":             p.Title,
		"content":          p.Body,
		"gv:proposalType":  p.ProposalType,
		"gv:status":        p.Status,
		"gv:durationHours": p.DurationHours,
		"published":        p.CreatedAt,
		"updated":          p.UpdatedAt,
		"context":          APID(domain, "nodes", p.NodeID),
	}
	if attributeAuthor {
		obj["attributedTo"] = APID(domain, "users", p.AuthorID)
	}
	if p.VotingEndsAt != nil {
		obj["gv:votingEndsAt"] = *p.VotingEndsAt
	}
	if p.TargetDoc != "" {
		obj["gv:targetDoc"] = p.TargetDoc
	}
	if p.ProposedBody != "" {
		obj["gv:proposedBody"] = p.ProposedBody
	}
	if p.GitSHA != "" {
		obj["gv:gitSha"] = p.GitSHA
	}
	return obj
}

// GovernanceDocToObject converts a GovernanceDoc to an AP gv:GovernanceDocument object.
func GovernanceDocToObject(doc model.GovernanceDoc, domain string, attributeAuthor bool) map[string]interface{} {
	obj := map[string]interface{}{
		"@context":   GovernanceContext(),
		"type":       "gv:GovernanceDocument",
		"id":         APID(domain, "governance", doc.ID),
		"name":       doc.Title,
		"content":    doc.Body,
		"gv:version": doc.Version,
		"published":  doc.CreatedAt,
		"updated":    doc.UpdatedAt,
		"context":    APID(domain, "nodes", doc.NodeID),
	}
	if attributeAuthor {
		obj["attributedTo"] = APID(domain, "users", doc.CreatedBy)
	}
	return obj
}

// VoteToActivity converts a Vote to an AP gv:Vote activity.
//
// This one carries an actor rather than an attribution, and the difference
// decides how a hidden membership is handled. A proposal minus its
// attribution is still the patch's text; a vote minus its actor is nothing
// but "somebody in this patch voted approve at 14:03", which against a
// followers collection on a small patch re-identifies the voter anyway. So
// the caller suppresses the broadcast outright rather than anonymizing it —
// see VoteOnProposal. The tally still federates when the proposal resolves,
// and ProposalResolvedActivity names nobody.
func VoteToActivity(v model.Vote, proposalAPID, voterAPID string) map[string]interface{} {
	return map[string]interface{}{
		"@context":  GovernanceContext(),
		"type":      "gv:Vote",
		"actor":     voterAPID,
		"object":    proposalAPID,
		"gv:value":  v.Value,
		"published": v.CreatedAt,
	}
}

// ProposalResolvedActivity creates a gv:ResolveProposal activity.
func ProposalResolvedActivity(proposalAPID, nodeAPID, result string, approveCount, rejectCount, abstainCount int) map[string]interface{} {
	return map[string]interface{}{
		"@context":        GovernanceContext(),
		"type":            "gv:ResolveProposal",
		"actor":           nodeAPID,
		"object":          proposalAPID,
		"gv:result":       result,
		"gv:approveCount": approveCount,
		"gv:rejectCount":  rejectCount,
		"gv:abstainCount": abstainCount,
	}
}
