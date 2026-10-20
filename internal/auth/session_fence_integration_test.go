//go:build integration

package auth

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/danyaa666/smemories/internal/db/dbtest"
)

// T-103 (audit run 1, lead 2): a login that read the old credential must not leave a session that survives a
// password reset or the Google pre-hijacking merge. The races are made deterministic by holding the only hashing slot:
// Login reads the user row, then waits for the slot; the credential change runs; the slot is released.

const (
	fenceOldPW = testPW
	fenceNewPW = "An0ther-fake-test-password-9!"
	fenceIP    = "192.0.2.1"
)

type fenceResult struct {
	sess *Session
	err  error
}

func newFenceEnv(t *testing.T) (e *env, uid uint64) {
	t.Helper()
	e = newEnv(t, opts{maxHashes: 1, hashWait: 30 * time.Second})
	want(t, e.register(testMail, fenceOldPW, "Alice"), 200, "")
	if err := e.db.QueryRow(`SELECT id FROM user_tab WHERE email = ?`, testMail).Scan(&uid); err != nil {
		t.Fatal(err)
	}
	return e, uid
}

// loginBlockedOnSlot holds the only hashing slot, starts Login (it reads the row, then waits for the slot) and returns
// the function that releases the slot and collects the result.
func (e *env) loginBlockedOnSlot(pw string) (finish func() fenceResult) {
	e.hasher.slots <- struct{}{}
	done := make(chan fenceResult, 1)
	go func() {
		_, s, err := e.svc.Login(context.Background(), fenceIP, "ua", testMail, pw, "")
		done <- fenceResult{s, err}
	}()
	time.Sleep(300 * time.Millisecond) // the goroutine has read the row and is parked in Hasher.acquire
	return func() fenceResult {
		<-e.hasher.slots
		select {
		case r := <-done:
			return r
		case <-time.After(20 * time.Second):
			e.t.Fatal("Login did not finish")
			return fenceResult{}
		}
	}
}

func (e *env) mustBeSignedOut(sess *Session, what string) {
	e.t.Helper()
	if _, _, err := e.svc.Authenticate(context.Background(), sess.Token); !errors.Is(err, ErrUnauthenticated) {
		e.t.Errorf("%s: Authenticate = %v, want ErrUnauthenticated", what, err)
	}
}

func (e *env) loginOK(email, pw string) *Session {
	e.t.Helper()
	_, s, err := e.svc.Login(context.Background(), fenceIP, "ua", email, pw, "")
	if err != nil {
		e.t.Fatalf("Login: %v", err)
	}
	return s
}

// Variant B: Login read the old hash and waits for a slot while the password is reset.
func TestLoginRacingPasswordResetLeavesNoSession(t *testing.T) {
	e, uid := newFenceEnv(t)
	ctx := context.Background()
	before := e.loginOK(testMail, fenceOldPW)
	phc, err := e.hasher.Hash(ctx, fenceNewPW)
	if err != nil {
		t.Fatal(err)
	}
	finish := e.loginBlockedOnSlot(fenceOldPW)
	if err := e.svc.store.resetPassword(ctx, uid, phc, e.clock.now(), e.sess.DeleteAll); err != nil {
		t.Fatal(err)
	}
	r := finish()
	if r.err != nil {
		t.Fatalf("the racing Login must have verified the old hash it read: %v", r.err)
	}
	e.mustBeSignedOut(before, "session from before the reset")
	e.mustBeSignedOut(r.sess, "session created by the login that raced the reset")
	if _, _, err := e.svc.Authenticate(ctx, e.loginOK(testMail, fenceNewPW).Token); err != nil {
		t.Errorf("login with the new password must work: %v", err)
	}
}

// Variant A: the same race against the Google pre-hijacking merge of an unverified account.
func TestLoginRacingGoogleMergeLeavesNoSession(t *testing.T) {
	e, _ := newFenceEnv(t)
	ctx := context.Background()
	before := e.loginOK(testMail, fenceOldPW)
	finish := e.loginBlockedOnSlot(fenceOldPW)
	_, google, err := e.svc.SignInGoogle(ctx, "ua", "", &GoogleIdentity{Subject: "s1", Email: testMail, Name: "V"})
	if err != nil {
		t.Fatal(err)
	}
	if n := e.count(`SELECT COUNT(*) FROM user_tab WHERE password_hash IS NULL AND email_verified_at IS NOT NULL`); n != 1 {
		t.Fatalf("the merge did not run: %d accounts are social-only and verified", n)
	}
	r := finish()
	if r.err != nil {
		t.Fatalf("the racing Login must have verified the old hash it read: %v", r.err)
	}
	e.mustBeSignedOut(before, "session from before the merge")
	e.mustBeSignedOut(r.sess, "session created by the login that raced the merge")
	if _, _, err := e.svc.Authenticate(ctx, google.Token); err != nil {
		t.Errorf("the Google session must work: %v", err)
	}
}

// Variant C: the login reads while the reset transaction is open (InnoDB returns the last committed row, the old hash).
func TestLoginDuringOpenResetTransactionLeavesNoSession(t *testing.T) {
	e, uid := newFenceEnv(t)
	ctx := context.Background()
	phc, err := e.hasher.Hash(ctx, fenceNewPW)
	if err != nil {
		t.Fatal(err)
	}
	var inWindow fenceResult
	err = e.svc.store.resetPassword(ctx, uid, phc, e.clock.now(), func(ctx context.Context, id uint64) error {
		if err := e.sess.DeleteAll(ctx, id); err != nil {
			return err
		}
		_, s, lerr := e.svc.Login(ctx, fenceIP, "ua", testMail, fenceOldPW, "")
		inWindow = fenceResult{s, lerr}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if inWindow.sess != nil {
		e.mustBeSignedOut(inWindow.sess, "session created while the reset transaction was open")
	}
}

// Control: a login that starts after the reset or the merge has finished is refused, so the fixture is sensitive.
func TestLoginAfterCredentialChangeIsRefused(t *testing.T) {
	e, uid := newFenceEnv(t)
	ctx := context.Background()
	phc, err := e.hasher.Hash(ctx, fenceNewPW)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.svc.store.resetPassword(ctx, uid, phc, e.clock.now(), e.sess.DeleteAll); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.svc.Login(ctx, fenceIP, "ua", testMail, fenceOldPW, ""); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("old password after reset: %v, want ErrInvalidCredentials", err)
	}
	if _, _, err := e.svc.SignInGoogle(ctx, "ua", "", &GoogleIdentity{Subject: "s1", Email: testMail, Name: "V"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.svc.Login(ctx, fenceIP, "ua", testMail, fenceNewPW, ""); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("password after the merge: %v, want ErrInvalidCredentials", err)
	}
}

// Sessions without an epoch field (issued before the deploy) count as epoch 0, so nobody is signed out by the migration.
func TestSessionWithoutEpochFieldIsEpochZero(t *testing.T) {
	e, _ := newFenceEnv(t)
	ctx := context.Background()
	s := e.loginOK(testMail, fenceOldPW)
	if err := e.rc.Client().HDel(ctx, e.rc.Key("sess", tokenKey(hashToken(s.Token))), "epoch").Err(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.svc.Authenticate(ctx, s.Token); err != nil {
		t.Fatalf("a session without an epoch field must keep working: %v", err)
	}
}

// A session whose epoch differs from the user's is refused and deleted.
func TestSessionWithStaleEpochIsRefusedAndDeleted(t *testing.T) {
	e, uid := newFenceEnv(t)
	ctx := context.Background()
	s := e.loginOK(testMail, fenceOldPW)
	if _, err := e.db.Exec(`UPDATE user_tab SET auth_epoch = auth_epoch + 1 WHERE id = ?`, uid); err != nil {
		t.Fatal(err)
	}
	e.mustBeSignedOut(s, "stale epoch")
	if n := e.sessionCount(); n != 0 {
		t.Errorf("%d sessions left in Redis, want the stale one deleted", n)
	}
	if _, _, err := e.svc.Authenticate(ctx, e.loginOK(testMail, fenceOldPW).Token); err != nil {
		t.Errorf("a session issued after the bump must work: %v", err)
	}
}

// AC3: the epoch comes with the user row Authenticate already reads, so a request costs one MySQL statement.
func TestAuthenticateStillCostsOneQuery(t *testing.T) {
	e, _ := newFenceEnv(t)
	s := e.loginOK(testMail, fenceOldPW)
	var n atomic.Int64
	e.svc.store = NewStore(dbtest.Spy(t, e.db, func(string) { n.Add(1) }))
	if _, _, err := e.svc.Authenticate(context.Background(), s.Token); err != nil {
		t.Fatal(err)
	}
	if got := n.Load(); got != 1 {
		t.Errorf("Authenticate issued %d MySQL statements, want 1", got)
	}
}
