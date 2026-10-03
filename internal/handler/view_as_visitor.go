package handler

import (
	"net/http"

	"github.com/patchwork-toolkit/patchwork/internal/middleware"
	"github.com/patchwork-toolkit/patchwork/internal/model"
)

// Looking at your own patch the way a stranger looks at it.
//
// There was no way to do that except to sign out. A board member came to
// check what she would be forwarding to somebody outside her organization,
// and the product's answer was to make her leave: "Let me look at my own
// page the way a stranger looks at it without signing out, because signing
// out is how I lost my account this afternoon." What she found by doing it
// anyway is a finding of its own, and the question she could not answer any
// other way — whether anything on her documents list was hidden — is the
// thing she came to be able to tell.
//
// # The rule
//
// `?as=visitor` on a GET makes this request's reader nobody. Every gate that
// asks who is reading asks through viewerOf below, so the answer is the one
// a signed-out visitor gets, computed by the same code rather than by a
// second implementation of "what the public sees" that could drift from the
// first. That is the whole point: a preview built from its own opinion of
// the rules would be a preview of the wrong thing.
//
// Three properties keep it safe, and all three are tested:
//
//  1. **It only ever subtracts.** The parameter's only effect is to drop the
//     viewer. There is no value of it, and no combination of it with
//     anything else, that reveals a row a caller could not already read;
//     the worst a bug here can do is show somebody less than they are
//     entitled to.
//  2. **It is ignored on anything but a GET.** Previewing is reading. A
//     write evaluated as "nobody" would refuse rather than escalate, so
//     this is belt and braces rather than a hole being closed — but it also
//     means nothing can act while wearing somebody else's standing, which
//     is the sentence worth being able to say plainly.
//  3. **It is nobody's secret.** Anyone may pass it, signed in or not, and
//     for a signed-out caller it changes nothing at all.
//
// It is deliberately a request parameter rather than a mode stored on the
// session. A stored mode is a thing that can be left on, and a person who
// leaves it on sees a patch they administer as a stranger does and concludes
// they have lost their access.
func viewingAsVisitor(r *http.Request) bool {
	if r.Method != http.MethodGet {
		return false
	}
	return r.URL.Query().Get("as") == "visitor"
}

// viewerOf is the reader a patch-scoped read should answer to: the signed-in
// user, or nobody where they have asked to see the patch as a visitor.
//
// Handlers that decide *what a reader may see* call this. Handlers that
// decide what somebody may *do* keep calling middleware.UserFromContext,
// because a preview must not be able to change what a write is allowed to
// do — in either direction.
func viewerOf(r *http.Request) *model.User {
	if viewingAsVisitor(r) {
		return nil
	}
	return middleware.UserFromContext(r.Context())
}
