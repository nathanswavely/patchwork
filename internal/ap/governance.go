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

// ProposalToObject converts a Proposal to an AP gv:Proposal object.
func ProposalToObject(p model.Proposal, domain string) map[string]interface{} {
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
		"attributedTo":     APID(domain, "users", p.AuthorID),
		"context":          APID(domain, "nodes", p.NodeID),
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
func GovernanceDocToObject(doc model.GovernanceDoc, domain string) map[string]interface{} {
	return map[string]interface{}{
		"@context":    GovernanceContext(),
		"type":        "gv:GovernanceDocument",
		"id":          APID(domain, "governance", doc.ID),
		"name":        doc.Title,
		"content":     doc.Body,
		"gv:version":  doc.Version,
		"published":   doc.CreatedAt,
		"updated":     doc.UpdatedAt,
		"attributedTo": APID(domain, "users", doc.CreatedBy),
		"context":     APID(domain, "nodes", doc.NodeID),
	}
}

// VoteToActivity converts a Vote to an AP gv:Vote activity.
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
