package auth

import (
	"encoding/base64"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/patchwork-toolkit/patchwork/internal/config"
)

// The one fingerprint this file trusts, computed here rather than taken from
// anybody's app: SHA-256 of the ASCII "patchwork-test-cert-6". Written in both
// spellings, the colon-separated hex a tool prints and the unpadded base64url
// a phone sends, so the derivation is checked against an answer that does not
// come from the code under test. It carries both '-' and '_', which is how a
// standard-base64 slip would be caught.
const (
	testFingerprint = "B2:7E:E8:4A:DC:DA:D8:69:F1:25:6F:73:64:1E:D2:A8:54:FB:05:AA:3F:F8:7E:90:F8:78:EF:2D:D1:0B:B4:05"
	testOrigin      = "android:apk-key-hash:sn7oStza2GnxJW9zZB7SqFT7Bao_-H6Q-HjvLdELtAU"
)

// The pair above is an arithmetic fact, so it is checked as one first. If this
// fails, every other expectation in this file is built on sand.
func TestNativeAppTestVectorIsSelfConsistent(t *testing.T) {
	raw, err := hex.DecodeString(strings.ReplaceAll(testFingerprint, ":", ""))
	if err != nil {
		t.Fatalf("test fingerprint is not hex: %v", err)
	}
	if len(raw) != 32 {
		t.Fatalf("a SHA-256 fingerprint is 32 bytes, this one is %d", len(raw))
	}
	want := "android:apk-key-hash:" + base64.RawURLEncoding.EncodeToString(raw)
	if want != testOrigin {
		t.Fatalf("test vector disagrees with itself: %s", want)
	}
}

func TestValidateAppleAppID(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"ABCDE12345.org.example.app", "ABCDE12345.org.example.app", true},
		{"  ABCDE12345.org.example.app  ", "ABCDE12345.org.example.app", true},
		{"1234567890.com.example.my-app", "1234567890.com.example.my-app", true},
		{"ABCDE12345.a", "ABCDE12345.a", true},
		// A Team ID is ten characters, uppercase and digits, and nothing else.
		{"ABCDE1234.org.example.app", "", false},
		{"ABCDE123456.org.example.app", "", false},
		{"abcde12345.org.example.app", "", false},
		{"ABCDE12345", "", false},
		{"ABCDE12345.", "", false},
		{"ABCDE12345.org.example app", "", false},
		{"", "", false},
	}
	for _, c := range cases {
		got, err := ValidateAppleAppID(c.in)
		if c.ok && err != nil {
			t.Errorf("ValidateAppleAppID(%q) refused a valid id: %v", c.in, err)
			continue
		}
		if !c.ok && err == nil {
			t.Errorf("ValidateAppleAppID(%q) accepted %q", c.in, got)
			continue
		}
		if got != c.want {
			t.Errorf("ValidateAppleAppID(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestValidateAndroidPackage(t *testing.T) {
	cases := []struct {
		in string
		ok bool
	}{
		{"org.example.app", true},
		{"  org.example.app  ", true},
		{"a.b", true},
		{"org.example_app.v2", true},
		{"Org.Example.App", true},
		// A package name needs at least one dot, and no segment may start
		// with a digit.
		{"orgexampleapp", false},
		{"org.1example", false},
		{"org..example", false},
		{"org.example.", false},
		{"org.example-app", false},
		{"", false},
	}
	for _, c := range cases {
		_, err := ValidateAndroidPackage(c.in)
		if c.ok != (err == nil) {
			t.Errorf("ValidateAndroidPackage(%q): err = %v, want ok = %v", c.in, err, c.ok)
		}
	}
}

func TestNormalizeFingerprintUppercasesAndChecksShape(t *testing.T) {
	lower := strings.ToLower(testFingerprint)
	got, err := NormalizeFingerprint("  " + lower + "  ")
	if err != nil {
		t.Fatalf("refused a lowercase fingerprint: %v", err)
	}
	if got != testFingerprint {
		t.Errorf("NormalizeFingerprint did not uppercase: %q", got)
	}

	for _, bad := range []string{
		"",
		"B2:7E",
		testFingerprint + ":00",
		strings.ReplaceAll(testFingerprint, ":", ""),
		strings.Replace(testFingerprint, "B2", "GG", 1),
		strings.Replace(testFingerprint, ":", "-", 1),
	} {
		if _, err := NormalizeFingerprint(bad); err == nil {
			t.Errorf("NormalizeFingerprint(%q) accepted a malformed fingerprint", bad)
		}
	}
}

// The derivation the whole Android half rests on: the colon-separated hex a
// developer pastes in, turned into the origin the phone actually sends.
func TestAPKKeyHashOriginMatchesTheKnownPair(t *testing.T) {
	got, err := APKKeyHashOrigin(testFingerprint)
	if err != nil {
		t.Fatalf("APKKeyHashOrigin: %v", err)
	}
	if got != testOrigin {
		t.Errorf("APKKeyHashOrigin(%s)\n got %s\nwant %s", testFingerprint, got, testOrigin)
	}

	// Case and surrounding space are a spelling, not a different key.
	lower, err := APKKeyHashOrigin(" " + strings.ToLower(testFingerprint) + " ")
	if err != nil || lower != testOrigin {
		t.Errorf("a lowercase fingerprint derived %q (%v), want the same origin", lower, err)
	}

	// The encoding is base64url and unpadded, so neither of the characters
	// standard base64 would have used may appear, and nor may '='.
	if strings.ContainsAny(got, "+/=") {
		t.Errorf("origin is not unpadded base64url: %s", got)
	}

	if _, err := APKKeyHashOrigin("not-a-fingerprint"); err == nil {
		t.Error("APKKeyHashOrigin accepted a string that is not a fingerprint")
	}
}

func TestSanitizeNativeAppLabel(t *testing.T) {
	if got := SanitizeNativeAppLabel("  Our phone app \n"); got != "Our phone app" {
		t.Errorf("label not trimmed and flattened: %q", got)
	}
	if got := SanitizeNativeAppLabel("Our\x00app"); got != "Ourapp" {
		t.Errorf("control character survived: %q", got)
	}
	// Unlike a passkey nickname, an empty label stays empty: the identifier
	// is already a name.
	if got := SanitizeNativeAppLabel("   "); got != "" {
		t.Errorf("empty label became %q", got)
	}
	long := strings.Repeat("é", NativeAppLabelMax+20)
	if got := SanitizeNativeAppLabel(long); len([]rune(got)) != NativeAppLabelMax {
		t.Errorf("label capped to %d runes, not %d", len([]rune(got)), NativeAppLabelMax)
	}
}

// An instance that has listed nothing accepts no extra origin. This is the
// default state and the one that must stay boring.
func TestNativeAppOriginsIsEmptyByDefault(t *testing.T) {
	db := setupTestDB(t)
	origins, err := NativeAppOrigins(db)
	if err != nil {
		t.Fatalf("NativeAppOrigins: %v", err)
	}
	if len(origins) != 0 {
		t.Errorf("a quilt that listed nothing accepts %v", origins)
	}
}

func TestNativeAppOriginsDerivesFromAndroidRowsOnly(t *testing.T) {
	db := setupTestDB(t)

	// An Apple row contributes nothing: an iOS assertion carries the https
	// origin the relying party already accepts.
	if _, err := db.Exec(
		`INSERT INTO native_apps (id, platform, identifier) VALUES (?, 'apple', 'ABCDE12345.org.example.app')`,
		NewUUIDv7()); err != nil {
		t.Fatal(err)
	}
	// Two Android rows, and the second repeats the first's fingerprint: one
	// key is one origin however many apps are signed with it.
	other := strings.Replace(testFingerprint, "B2", "C3", 1)
	if _, err := db.Exec(
		`INSERT INTO native_apps (id, platform, identifier, fingerprints) VALUES (?, 'android', 'org.example.app', ?)`,
		NewUUIDv7(), `["`+testFingerprint+`","`+other+`"]`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(
		`INSERT INTO native_apps (id, platform, identifier, fingerprints) VALUES (?, 'android', 'org.example.other', ?)`,
		NewUUIDv7(), `["`+testFingerprint+`"]`); err != nil {
		t.Fatal(err)
	}

	origins, err := NativeAppOrigins(db)
	if err != nil {
		t.Fatalf("NativeAppOrigins: %v", err)
	}
	if len(origins) != 2 {
		t.Fatalf("got %d origins, want 2: %v", len(origins), origins)
	}
	if origins[0] != testOrigin {
		t.Errorf("first origin %q, want %q", origins[0], testOrigin)
	}
	for _, o := range origins {
		if !strings.HasPrefix(o, "android:apk-key-hash:") {
			t.Errorf("origin %q is not an apk-key-hash origin", o)
		}
	}
}

// A row that somehow holds nonsense must not be able to stop a boot: the
// write paths check the shape, and this is the belt for the braces.
func TestNativeAppOriginsSkipsUnreadableRows(t *testing.T) {
	db := setupTestDB(t)
	if _, err := db.Exec(
		`INSERT INTO native_apps (id, platform, identifier, fingerprints) VALUES (?, 'android', 'org.example.broken', 'not json')`,
		NewUUIDv7()); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(
		`INSERT INTO native_apps (id, platform, identifier, fingerprints) VALUES (?, 'android', 'org.example.app', ?)`,
		NewUUIDv7(), `["nope","`+testFingerprint+`"]`); err != nil {
		t.Fatal(err)
	}
	origins, err := NativeAppOrigins(db)
	if err != nil {
		t.Fatalf("NativeAppOrigins: %v", err)
	}
	if len(origins) != 1 || origins[0] != testOrigin {
		t.Errorf("got %v, want just %q", origins, testOrigin)
	}
}

// Reconfigure swaps the relying party's origin list and leaves the ceremony
// store alone.
func TestReconfigureAddsOriginsAndKeepsSessions(t *testing.T) {
	db := setupTestDB(t)
	cfg := &config.Config{}
	cfg.Instance.Name = "Test Quilt"
	cfg.Instance.Domain = "quilt.example"

	svc, err := NewWebAuthnService(db, cfg)
	if err != nil {
		t.Fatalf("NewWebAuthnService: %v", err)
	}
	if got := svc.engine().Config.RPOrigins; len(got) != 1 || got[0] != "https://quilt.example" {
		t.Fatalf("a fresh service accepts %v", got)
	}

	svc.sessions.Set("ceremony", nil)

	if err := svc.Reconfigure([]string{testOrigin}); err != nil {
		t.Fatalf("Reconfigure: %v", err)
	}
	got := svc.engine().Config.RPOrigins
	if len(got) != 2 || got[0] != "https://quilt.example" || got[1] != testOrigin {
		t.Errorf("after Reconfigure the origins are %v", got)
	}
	if svc.engine().Config.RPID != "quilt.example" {
		t.Errorf("Reconfigure changed the RP ID to %q", svc.engine().Config.RPID)
	}
	if _, ok := svc.sessions.Get("ceremony"); !ok {
		t.Error("Reconfigure dropped a ceremony that was in flight")
	}

	// And back to none, which is what removing the last Android app does.
	if err := svc.Reconfigure(nil); err != nil {
		t.Fatalf("Reconfigure back: %v", err)
	}
	if got := svc.engine().Config.RPOrigins; len(got) != 1 {
		t.Errorf("removing the last app left %v", got)
	}
}
