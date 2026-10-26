package class

import (
	"time"

	"github.com/danyaa666/smemories/internal/apperr"
)

// Business codes of the class endpoints beyond the field codes of invalid (docs/api-contract.md section 3).
var (
	// CodeLimitReached: the user is already admin of maxAdminClasses classes (409).
	CodeLimitReached = apperr.NewCode("ERROR_LIMIT_REACHED", 409)
	// CodeEmailNotVerified: creating a class needs a verified email (403).
	CodeEmailNotVerified = apperr.NewCode("ERROR_EMAIL_NOT_VERIFIED", 403)
	// CodeLastAdmin: the last admin of a class can neither leave, be removed nor be demoted (403).
	CodeLastAdmin = apperr.NewCode("ERROR_LAST_ADMIN", 403)
	// CodeClassFull: the class already has MaxMembers members, so an invite cannot be accepted (409).
	CodeClassFull = apperr.NewCode("ERROR_CLASS_FULL", 409)
)

var (
	// ErrNotFound covers a class that does not exist and one the caller is not a member of: callers cannot tell them apart,
	// and the two answers are byte-identical apart from the request id.
	ErrNotFound = apperr.New(apperr.NotFound, "class not found")
	// ErrMemberNotFound is an admin naming a member that is not in this class.
	ErrMemberNotFound = apperr.New(apperr.NotFound, "member not found")
	// ErrForbidden is a member whose role is too low for the action.
	ErrForbidden = apperr.New(apperr.Forbidden, "this action needs the admin role")
	// ErrLimitReached is the 11th class as admin, whether by creating one or by being promoted in one.
	ErrLimitReached = apperr.New(CodeLimitReached, "at most 10 classes as admin per user")
	// ErrInviteLimit is the 6th active invite of a class.
	ErrInviteLimit = apperr.New(CodeLimitReached, "at most 5 active invites per class")
	// ErrClassFull is accepting an invite into a class that has MaxMembers members.
	ErrClassFull = apperr.New(CodeClassFull, "the class is full")
	// ErrInviteNotFound is the one answer for a token that is unknown, malformed, expired or revoked, and for an invite id that is
	// not in this class: the caller cannot tell them apart, and the bodies are byte-identical apart from the request id.
	ErrInviteNotFound = apperr.New(apperr.NotFound, "invite not found")
	// ErrEmailNotVerified is class/create or class-invite/accept from an account whose email is not verified yet.
	ErrEmailNotVerified = apperr.New(CodeEmailNotVerified, "verify your email first")
	// ErrLastAdmin protects the class from being left without an admin.
	ErrLastAdmin = apperr.New(CodeLastAdmin, "a class needs at least one admin")
)

// RetryAfterError is a 429: the handler turns After into the Retry-After header.
type RetryAfterError struct {
	Err   error
	After time.Duration
}

func (e *RetryAfterError) Error() string { return e.Err.Error() }
func (e *RetryAfterError) Unwrap() error { return e.Err }
