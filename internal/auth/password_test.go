package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

var fastParams = HashParams{MemoryKiB: 64, Time: 1, Parallelism: 1} // tests only; production uses config defaults

func testHasher() *Hasher { return NewHasher(fastParams, 4, 2*time.Second) }

func TestHashVerifyRoundTrip(t *testing.T) {
	h := testHasher()
	ctx := context.Background()
	phc, err := h.Hash(ctx, "correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(phc, "$argon2id$v=19$m=64,t=1,p=1$") || strings.Contains(phc, "correct") {
		t.Fatalf("not a PHC argon2id string: %q", phc)
	}
	if ok, err := h.Verify(ctx, "correct horse battery", phc); !ok || err != nil {
		t.Fatalf("right password: %v %v", ok, err)
	}
	if ok, err := h.Verify(ctx, "correct horse batterY", phc); ok || err != nil {
		t.Fatalf("wrong password: %v %v", ok, err)
	}
	other, _ := h.Hash(ctx, "correct horse battery")
	if other == phc {
		t.Fatal("salt must be random")
	}
}

func TestDefaultParametersAreEncoded(t *testing.T) {
	h := NewHasher(HashParams{MemoryKiB: 19456, Time: 2, Parallelism: 1}, 1, time.Second)
	phc, err := h.Hash(context.Background(), "some password")
	if err != nil || !strings.HasPrefix(phc, "$argon2id$v=19$m=19456,t=2,p=1$") {
		t.Fatalf("%q %v", phc, err)
	}
	// salt 16 bytes -> 22 chars, key 32 bytes -> 43 chars (raw base64)
	parts := strings.Split(phc, "$")
	if len(parts[4]) != 22 || len(parts[5]) != 43 {
		t.Fatalf("salt/key sizes wrong: %d %d", len(parts[4]), len(parts[5]))
	}
}

func TestVerifyUsesStoredParams(t *testing.T) {
	ctx := context.Background()
	phc, _ := testHasher().Hash(ctx, "pw-for-old-params")
	retuned := NewHasher(HashParams{MemoryKiB: 128, Time: 2, Parallelism: 2}, 1, time.Second)
	if ok, err := retuned.Verify(ctx, "pw-for-old-params", phc); !ok || err != nil {
		t.Fatalf("hash made under old params must still verify: %v %v", ok, err)
	}
}

func TestVerifyRejectsMalformedHashes(t *testing.T) {
	h := testHasher()
	for _, bad := range []string{
		"", "plaintext", "$argon2i$v=19$m=64,t=1,p=1$c2FsdHNhbHQ$a2V5a2V5",
		"$argon2id$v=18$m=64,t=1,p=1$c2FsdHNhbHQ$a2V5a2V5",
		"$argon2id$v=19$m=0,t=1,p=1$c2FsdHNhbHQ$a2V5a2V5",
		"$argon2id$v=19$m=99999999,t=1,p=1$c2FsdHNhbHQ$a2V5a2V5",
		"$argon2id$v=19$m=64,t=1,p=1$!!!$a2V5a2V5",
		"$argon2id$v=19$m=64,t=1,p=1$c2FsdHNhbHQ$",
		"$argon2id$v=19$m=64,t=1,p=1$c2FsdHNhbHQ$" + strings.Repeat("QQ", 1025), // key > maxStoredKeyLen
	} {
		if ok, err := h.Verify(context.Background(), "x", bad); ok || err == nil {
			t.Errorf("%q: want an error, got ok=%v err=%v", bad, ok, err)
		}
	}
}

func TestBusyWhenNoSlotWithinWait(t *testing.T) {
	h := NewHasher(fastParams, 1, 50*time.Millisecond)
	h.slots <- struct{}{} // the only slot is taken
	start := time.Now()
	if _, err := h.Hash(context.Background(), "pw"); !errors.Is(err, ErrBusy) {
		t.Fatalf("Hash: %v, want ErrBusy", err)
	}
	if _, err := h.Verify(context.Background(), "pw", "$argon2id$v=19$m=64,t=1,p=1$c2FsdHNhbHQ$a2V5a2V5"); !errors.Is(err, ErrBusy) {
		t.Fatalf("Verify: %v, want ErrBusy", err)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("did not give up promptly")
	}
	<-h.slots
	if _, err := h.Hash(context.Background(), "pw"); err != nil {
		t.Fatalf("slot free again: %v", err)
	}
}

func TestSlotsAreReleasedUnderConcurrency(t *testing.T) {
	h := NewHasher(HashParams{MemoryKiB: 4096, Time: 1, Parallelism: 1}, 2, 10*time.Second)
	errs := make(chan error, 12)
	for i := 0; i < 12; i++ {
		go func() { _, err := h.Hash(context.Background(), "pw"); errs <- err }()
	}
	for i := 0; i < 12; i++ {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	if len(h.slots) != 0 {
		t.Fatalf("slots leaked: %d", len(h.slots))
	}
}
