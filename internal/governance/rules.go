package governance

import (
	"encoding/json"
	"fmt"

	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/model"
)

// GovernanceRules is the complete governance configuration stored in governance-rules.json.
type GovernanceRules struct {
	// Decision making
	DecisionMethod      string                    `json:"decision_method"`
	QuorumPercent       int                       `json:"quorum_percent"`
	DefaultVoteDuration int                       `json:"default_vote_duration_hours"`
	AmendmentThreshold  string                    `json:"amendment_threshold"`
	AmendmentAutoApply  bool                      `json:"amendment_auto_apply"`
	MinVotingTenureDays int                       `json:"min_voting_tenure_days"`

	// Leadership & succession
	LeadershipModel     string                    `json:"leadership_model"`
	SuccessionMethod    string                    `json:"succession_method"`
	SuccessionPolicy    string                    `json:"succession_policy"`
	AdminTermMonths     int                       `json:"admin_term_months"`
	MaxAdmins           int                       `json:"max_admins"`
	InactivityDays      int                       `json:"inactivity_days"`

	// Membership
	MembershipPolicy    string                    `json:"membership_policy"`
	FollowerPermissions model.FollowerPermissions  `json:"follower_permissions"`
}

// DefaultRules returns the default governance rules.
func DefaultRules() *GovernanceRules {
	return &GovernanceRules{
		DecisionMethod:      "majority",
		QuorumPercent:       0,
		DefaultVoteDuration: 72,
		AmendmentThreshold:  "majority",
		AmendmentAutoApply:  true,
		MinVotingTenureDays: 0,
		LeadershipModel:     "maintainer",
		SuccessionMethod:    "admin_nominate",
		SuccessionPolicy:    "longest_tenure",
		AdminTermMonths:     0,
		MaxAdmins:           3,
		InactivityDays:      90,
		MembershipPolicy:    "open",
		FollowerPermissions: model.FollowerPermissions{
			Events:    true,
			Proposals: true,
			Charters:  true,
			Members:   true,
		},
	}
}

// ReadRules reads the governance rules from the git repo for a node.
// Returns defaults if the file doesn't exist or can't be parsed.
func ReadRules(dataDir, nodeID string) (*GovernanceRules, error) {
	content, err := GetDocument(dataDir, nodeID, "governance-rules.json")
	if err != nil {
		return DefaultRules(), nil
	}

	rules := DefaultRules()
	if err := json.Unmarshal([]byte(content), rules); err != nil {
		return DefaultRules(), nil
	}

	return rules, nil
}

// WriteRules marshals rules to governance-rules.json and commits it directly
// to the node repo's main branch. Returns the commit SHA. The write is
// skipped (empty SHA, nil error) when the file already holds this content,
// so callers can invoke it unconditionally without minting no-op commits.
func WriteRules(dataDir, nodeID string, rules *GovernanceRules, message string) (string, error) {
	b, err := json.MarshalIndent(rules, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal rules: %w", err)
	}
	content := string(b) + "\n"
	if current, err := GetDocument(dataDir, nodeID, "governance-rules.json"); err == nil && current == content {
		return "", nil
	}
	return DirectEdit(dataDir, nodeID, "governance-rules.json", content, "Patchwork System", "system@patchwork.local", message)
}

// marshalConfig renders the governance_config cache column value for a rule set.
func marshalConfig(rules *GovernanceRules) (string, error) {
	gcJSON, err := json.Marshal(model.GovernanceConfig{
		DecisionMethod:      rules.DecisionMethod,
		QuorumPercent:       rules.QuorumPercent,
		DefaultVoteDuration: rules.DefaultVoteDuration,
		AmendmentThreshold:  rules.AmendmentThreshold,
		AmendmentAutoApply:  rules.AmendmentAutoApply,
		SuccessionPolicy:    rules.SuccessionPolicy,
		MinVotingTenureDays: rules.MinVotingTenureDays,
		LeadershipModel:     rules.LeadershipModel,
		SuccessionMethod:    rules.SuccessionMethod,
		AdminTermMonths:     rules.AdminTermMonths,
		MaxAdmins:           rules.MaxAdmins,
		InactivityDays:      rules.InactivityDays,
	})
	if err != nil {
		return "", fmt.Errorf("marshal governance_config: %w", err)
	}
	return string(gcJSON), nil
}

// SyncConfigToDB writes only the governance_config cache column from the
// node's git rules (defaults when no repo exists). Unlike SyncRulesToDB it
// leaves membership_policy and follower_permissions alone — at creation
// those are the creator's form choices, which a template must not override.
func SyncConfigToDB(db *database.DB, dataDir, nodeID string) error {
	rules, err := ReadRules(dataDir, nodeID)
	if err != nil {
		return err
	}
	gcJSON, err := marshalConfig(rules)
	if err != nil {
		return err
	}
	_, err = db.Exec("UPDATE nodes SET governance_config = ? WHERE id = ?", gcJSON, nodeID)
	return err
}

// SyncRulesToDB syncs the governance rules from git into the database cache columns.
func SyncRulesToDB(db *database.DB, dataDir, nodeID string) error {
	rules, err := ReadRules(dataDir, nodeID)
	if err != nil {
		return err
	}

	gcJSON, err := marshalConfig(rules)
	if err != nil {
		return err
	}

	fpJSON, err := json.Marshal(rules.FollowerPermissions)
	if err != nil {
		return fmt.Errorf("marshal follower_permissions: %w", err)
	}

	_, err = db.Exec(
		"UPDATE nodes SET governance_config = ?, membership_policy = ?, follower_permissions = ? WHERE id = ?",
		gcJSON, rules.MembershipPolicy, string(fpJSON), nodeID,
	)
	return err
}
