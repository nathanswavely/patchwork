package handler

// The deletion boundary (docs/adr/086, generalised).
//
// Deleting an account keeps the users row as a tombstone, so **every foreign
// key declared ON DELETE CASCADE never fires**. Whatever a person's rows are
// supposed to do when they leave has to be written down and carried out by
// hand — the ADR does exactly that for seats.holder_id, and the purge list in
// account_deletion.go does it for the tables that go.
//
// A list maintained by hand fails silently when somebody adds a table and
// does not think of it: nothing errors, the rows simply stay. docs/adr/083's
// contact_items was added that way and left a person's phone number in the
// database after they deleted their account. The seamrip boundary has had a
// test against this failure mode since migration 050's seats
// (TestEveryTableHasABoundaryDecision) and the member view since docs/adr/089
// (TestEveryTableHasAMemberViewRule). This is the same guard for the third
// list.
//
// The rules are declared here rather than in account_deletion.go so the
// question reads as its own thing: not "what does the purge loop run", but
// "what happens to this table when a person leaves".

// deletionDisposition is what a self-serve deletion does with one table.
type deletionDisposition int

const (
	// purged: the rows are the person alone and are deleted outright.
	purged deletionDisposition = iota
	// kept: the rows are a community record that outlives the person, who
	// remains as a tombstone reference — a vote still has a voter.
	kept
	// emptied: the rows stay and the person's own fields in them are
	// cleared, the way the users row itself is.
	emptied
)

// deletionAlsoPersonal names tables whose rows are about a person through a
// reference the schema does not declare as a foreign key, so the boundary
// test's walk cannot find them. Each still needs a rule; this is only how the
// test learns to ask.
func deletionAlsoPersonal() map[string]string {
	return map[string]string{
		"ap_followers": "local_actor_id is polymorphic — local_actor_type says whether it names a user or a node — so there is no foreign key to follow.",
	}
}

// deletionRule is one table's answer, with the reasoning that makes it one.
type deletionRule struct {
	Disposition deletionDisposition
	// Why is prose and is required: it is what the boundary test checks for,
	// and what a reader comes here to find.
	Why string
}

// deletionRules maps a table to what deletion does with it. Every table whose
// rows are about one person needs an entry — see personalTables in the test
// for how that set is derived from the schema rather than listed here.
func deletionRules() map[string]deletionRule {
	return map[string]deletionRule{
		// Credentials and delivery state: the person is the only referent.
		"sessions":                 {purged, "a session is the person signed in; there is nobody left to be signed in as."},
		"credentials":              {purged, "a passkey authenticates one person and authenticates nobody once they are gone."},
		"recovery_codes":           {purged, "a way back into an account that no longer opens."},
		"notifications":            {purged, "delivery state for a reader who will not read."},
		"notification_preferences": {purged, "how to reach somebody who has asked not to be reached at all."},

		// Relationships that end when the person does.
		"memberships":         {purged, "standing in a community is a relationship, and it ends. Removing it is also what ends every contact disclosure (docs/adr/083)."},
		"user_quilts":         {purged, "the person's own list of other quilts."},
		"remote_follows":      {purged, "a follow the person made, relayed by the instance actor on their behalf."},
		"label_stewards":      {purged, "a steward listing names a person to contact."},
		"ap_followers":        {purged, "an inbox to deliver to, for an actor that is now gone."},
		"contact_items":       {purged, "a contact card is the person, never an act. Ending the disclosure is not erasing the value, and CASCADE cannot fire against a tombstone (docs/adr/083)."},
		"contact_item_shares": {purged, "with the items they disclose."},

		// Curation and provenance: the row is an instance or patch record
		// and the person is who did it, which is exactly what a record is
		// for. None of these was decided before this boundary existed; each
		// states the behaviour that already ran.
		"aggregators":                 {kept, "instance-level crosswalk curation (docs/adr/056); added_by is provenance for a record the instance keeps."},
		"aggregator_programs":         {kept, "a crediting decision, kept with its aggregator."},
		"aggregator_ignored_names":    {kept, "a curation decision, kept with its aggregator."},
		"aggregator_offer_dismissals": {kept, "a curation decision about an event, not a fact about the person."},
		"event_sources":               {kept, "a feed belongs to the patch it was attached to; added_by is provenance."},
		"governance_docs":             {kept, "a charter is the community's text; created_by is provenance and the document outlives everyone."},
		"invite_links":                {kept, "instance-scoped and single-use (docs/adr/001); created_by is provenance. NOTE: this records what already happens rather than a decision anybody made — an unspent link outliving the admin who issued it may deserve a second look, and this boundary is where that argument belongs."},

		// Standing offers, which an absent person cannot honour.
		"election_candidates": {purged, "a candidacy is an offer to serve, and a tombstone must not win a seat."},
		"claim_requests":      {purged, "a pending claim asks an admin to hand a patch to somebody who will not receive it. Settled claims are a record of a review and are kept, minus the address they carried."},

		// Acts of record. The person becomes a tombstone reference and the
		// row stays: this is the whole design (docs/adr/086).
		"proposals":              {kept, "a proposal is an act; the community's record cannot lose its author."},
		"votes":                  {kept, "a ballot is a decision that stays counted."},
		"proposal_comments":      {kept, "said in a room, and the room's record keeps it."},
		"comment_reactions":      {kept, "part of the same record."},
		"proposal_revisions":     {kept, "the history of a text the community edited."},
		"events":                 {kept, "an event happened; who posted it is provenance."},
		"notices":                {kept, "a notice is addressed to a room and stays in it (docs/adr/081)."},
		"notice_replies":         {kept, "part of the notice's record."},
		"nodes":                  {kept, "a patch outlives its creator; deletion is refused while they are its only admin."},
		"seats":                  {kept, "a seat outlives its holder; holder_id is nulled by hand because CASCADE cannot fire (docs/adr/086)."},
		"attestations":           {kept, "a community's statement about a decision it made."},
		"attestation_names":      {kept, "the names that statement carried."},
		"amendment_attestations": {kept, "a text a meeting adopted."},
		"audit_log":              {kept, "the record of what was done, including the deletion itself."},
		"content_reports":        {kept, "a report is a moderation record; the queue must not lose its history."},
		"event_links":            {kept, "a link between an event and a patch, neither of which is the person."},
		"election_ballots":       {kept, "an approval vote that stays counted."},

		// The row stays and the person's own fields in it are cleared.
		"users": {emptied, "the tombstone: the row is kept so every RESTRICT holds, the identity columns are emptied, and the username is retired rather than freed."},
	}
}

// deletionPurges is the statement list the deletion runs, in order. It lives
// here beside the rules so the decision and its execution are read together,
// and so the boundary test can check that a table called purged is one.
// Every statement binds the person's id once.
func deletionPurges() []string {
	return []string{
		`DELETE FROM sessions WHERE user_id = ?`,
		`DELETE FROM credentials WHERE user_id = ?`,
		`DELETE FROM recovery_codes WHERE user_id = ?`,
		`DELETE FROM notifications WHERE user_id = ?`,
		`DELETE FROM notification_preferences WHERE user_id = ?`,
		`DELETE FROM memberships WHERE user_id = ?`,
		`DELETE FROM user_quilts WHERE user_id = ?`,
		`DELETE FROM remote_follows WHERE user_id = ?`,
		`DELETE FROM label_stewards WHERE user_id = ?`,
		`DELETE FROM election_candidates WHERE user_id = ?`,
		`DELETE FROM ap_followers WHERE local_actor_type = 'user' AND local_actor_id = ?`,
		`DELETE FROM claim_requests WHERE user_id = ? AND status = 'pending'`,
		// Shares first, though the FK would cascade them: this list is the
		// statement of what goes, and a reader should not have to know the
		// schema to see that it goes.
		`DELETE FROM contact_item_shares WHERE item_id IN (SELECT id FROM contact_items WHERE user_id = ?)`,
		`DELETE FROM contact_items WHERE user_id = ?`,
	}
}

// DeletionRuleView is deletionRule as the boundary test sees it.
type DeletionRuleView struct {
	IsPurged bool
	Why      string
}

// DeletionRules exposes the boundary for the test in handler_test.
func DeletionRules() map[string]DeletionRuleView {
	out := map[string]DeletionRuleView{}
	for name, r := range deletionRules() {
		out[name] = DeletionRuleView{IsPurged: r.Disposition == purged, Why: r.Why}
	}
	return out
}

// DeletionAlsoPersonal exposes the escape hatch for the boundary test.
func DeletionAlsoPersonal() map[string]string { return deletionAlsoPersonal() }

// DeletionPurgeStatements exposes the purge list for the boundary test.
func DeletionPurgeStatements() []string { return deletionPurges() }
