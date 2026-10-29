package handler

import (
	"net/http"

	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/middleware"
	"github.com/patchwork-toolkit/patchwork/internal/settings"
)

// A post says whether AI made it
// (docs/adr/2026-10-20-a-post-says-whether-ai-made-it.md).
//
// The surfaces that publish (events, patches, proposals, charter edits,
// notices) ask one question with three answers, and images are asked
// separately. Nothing is preselected and silence is never an answer: an
// unanswered question is stored as NULL, which is also what everything
// written elsewhere carries. Only `generated` is ever shown to readers.
//
// The instance switch decides whether the question is asked at all. In
// `required` a create without an answer is a 400 naming the field, so an API
// client cannot skip what the form asks. Edits are not refused for a missing
// answer: existing rows are NULL, and on a required quilt the form asks the
// next time someone edits one.

const (
	aiUseNone      = "none"
	aiUseAssisted  = "assisted"
	aiUseGenerated = "generated"
)

func validAIUse(v string) bool {
	return v == aiUseNone || v == aiUseAssisted || v == aiUseGenerated
}

// checkAIAnswer validates one answer on a create. field names it in the
// error. nil is unanswered: fine unless the quilt requires an answer. A
// non-nil value must be one of the three, on any setting, because a stored
// answer is only worth something if it means one thing.
func checkAIAnswer(db *database.DB, field string, v *string) string {
	if v == nil || *v == "" {
		if settings.AIDisclosure(db) == settings.AIDisclosureRequired {
			return field + " is required: say whether AI was used (none, assisted or generated)"
		}
		return ""
	}
	if !validAIUse(*v) {
		return field + " must be none, assisted or generated"
	}
	return ""
}

// checkImageAIAnswer is checkAIAnswer for an image's answer, which is only
// owed when there is an image. Without one an answer is dropped rather than
// stored, so a cleared image never leaves a label behind on nothing.
func checkImageAIAnswer(db *database.DB, field, imageURL string, v *string) (string, *string) {
	if imageURL == "" {
		return "", nil
	}
	return checkAIAnswer(db, field, v), aiAnswerValue(v)
}

// aiAnswerValue is the value to store: NULL for unanswered.
func aiAnswerValue(v *string) *string {
	if v == nil || *v == "" {
		return nil
	}
	return v
}

// checkPatchedAIAnswer validates an answer arriving in a partial PATCH. Absent
// leaves the stored answer alone; null or "" clears it, except on a quilt that
// requires one, where withdrawing an answer would undo what the create was
// made to give. The value is normalized in req so the caller writes NULL
// rather than "".
func checkPatchedAIAnswer(db *database.DB, field string, req map[string]interface{}) string {
	raw, present := req[field]
	if !present {
		return ""
	}
	if raw == nil {
		if settings.AIDisclosure(db) == settings.AIDisclosureRequired {
			return field + " is required: say whether AI was used (none, assisted or generated)"
		}
		return ""
	}
	v, ok := raw.(string)
	if !ok {
		return field + " must be none, assisted or generated"
	}
	if v == "" {
		req[field] = nil
		if settings.AIDisclosure(db) == settings.AIDisclosureRequired {
			return field + " is required: say whether AI was used (none, assisted or generated)"
		}
		return ""
	}
	if !validAIUse(v) {
		return field + " must be none, assisted or generated"
	}
	return ""
}

// hideAIWork reports whether this viewer has asked to hide patches that
// declare their work relies on AI from discovery (decision 6). Per person
// only: there is no instance-wide twin, because the reference policy welcomes
// labelled work. Like the amended-lining filter it never applies to My Quilt,
// and a direct link always works.
func hideAIWork(db *database.DB, r *http.Request) bool {
	user := middleware.UserFromContext(r.Context())
	if user == nil {
		return false
	}
	var hide int
	if err := db.QueryRow("SELECT hide_ai_work FROM users WHERE id = ?", user.ID).Scan(&hide); err != nil {
		return false
	}
	return hide == 1
}

// reasonUnlabelledAI is the report reason for content that looks AI-made and
// says nothing (decision 7). The other reasons are stored as the words the
// reporter picked; this one is a key, because the moderation queue offers a
// remedy for it that it offers for nothing else.
const reasonUnlabelledAI = "unlabelled_ai"

// reportReasonText is a report's reason in words, for a notification.
func reportReasonText(reason string) string {
	if reason == reasonUnlabelledAI {
		return "AI-made content without a label"
	}
	return reason
}

// labelTable names the table a label action writes for a reported entity,
// and whether that entity has the column at all: an image answer exists only
// on patches and events, and a reply or a person carries no answer.
// Literals only, since the result is interpolated into SQL.
func labelTable(entityType, column string) (string, bool) {
	switch entityType {
	case "node":
		return "nodes", true
	case "event":
		return "events", true
	case "proposal":
		return "proposals", column == "ai_use"
	case "notice":
		return "notices", column == "ai_use"
	}
	return "", false
}
