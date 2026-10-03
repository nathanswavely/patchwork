package auth

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/patchwork-toolkit/patchwork/internal/database"
)

// Native apps: the identifiers an instance admin has said this domain
// vouches for (docs/adr/2026-09-20-an-instance-vouches-for-an-app.md).
//
// Everything here is shape-checking and derivation. Nothing in this file
// knows a single app identifier, and nothing ever will: the project names no
// app, and an instance that has listed nothing publishes nothing.

// appleAppIDPattern is "TEAMID.bundle.id" — a ten-character Team ID, a dot,
// then a bundle identifier. Apple's own files spell it exactly this way, and
// the string goes verbatim into the webcredentials list, so it is checked
// before it is stored rather than on the way out.
var appleAppIDPattern = regexp.MustCompile(`^[A-Z0-9]{10}\.[A-Za-z0-9.-]+$`)

// androidPackagePattern is a Java package name: at least two dot-separated
// segments, each starting with a letter.
var androidPackagePattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]*(\.[a-zA-Z][a-zA-Z0-9_]*)+$`)

// fingerprintPattern is a SHA-256 certificate fingerprint as every Android
// tool prints one: 32 uppercase hex pairs joined by colons.
var fingerprintPattern = regexp.MustCompile(`^([0-9A-F]{2}:){31}[0-9A-F]{2}$`)

// NativeAppLabelMax caps what an admin calls an app in the list. Same number
// as a passkey nickname, for the same reason: long enough to be a name,
// short enough that the list stays a list.
const NativeAppLabelMax = 64

// ValidateAppleAppID trims and checks an Apple app identifier, returning it
// as it will be stored.
func ValidateAppleAppID(s string) (string, error) {
	s = strings.TrimSpace(s)
	if !appleAppIDPattern.MatchString(s) {
		return "", fmt.Errorf("an Apple identifier is a ten-character Team ID, a dot, and a bundle id (ABCDE12345.org.example.app)")
	}
	return s, nil
}

// ValidateAndroidPackage trims and checks an Android package name.
func ValidateAndroidPackage(s string) (string, error) {
	s = strings.TrimSpace(s)
	if !androidPackagePattern.MatchString(s) {
		return "", fmt.Errorf("an Android identifier is a package name (org.example.app)")
	}
	return s, nil
}

// NormalizeFingerprint trims and uppercases a SHA-256 signing-certificate
// fingerprint and checks its shape. Uppercasing first is deliberate: the
// tools that print these disagree about case, and two spellings of one
// fingerprint would derive two origins and list the app twice.
func NormalizeFingerprint(s string) (string, error) {
	s = strings.ToUpper(strings.TrimSpace(s))
	if !fingerprintPattern.MatchString(s) {
		return "", fmt.Errorf("a signing-certificate fingerprint is 32 hex pairs joined by colons (AA:BB:…)")
	}
	return s, nil
}

// SanitizeNativeAppLabel cleans the name an admin gives an app, exactly the
// way SanitizeCredentialName cleans a passkey nickname — except that an
// empty label is allowed to stay empty, because the identifier is already a
// name and an unlabelled row is not a defect.
func SanitizeNativeAppLabel(label string) string {
	cleaned := sanitizeDisplayText(label)
	if runes := []rune(cleaned); len(runes) > NativeAppLabelMax {
		cleaned = strings.TrimSpace(string(runes[:NativeAppLabelMax]))
	}
	return cleaned
}

// APKKeyHashOrigin turns a signing-certificate fingerprint into the origin an
// Android app's passkey assertion actually carries:
// "android:apk-key-hash:<base64url of the raw 32 SHA-256 bytes, unpadded>".
//
// The colon-separated hex is the same 32 bytes the phone hashes; the two
// spellings exist because one is for people and one is for the protocol.
func APKKeyHashOrigin(fingerprint string) (string, error) {
	normalized, err := NormalizeFingerprint(fingerprint)
	if err != nil {
		return "", err
	}
	raw, err := hex.DecodeString(strings.ReplaceAll(normalized, ":", ""))
	if err != nil {
		return "", fmt.Errorf("fingerprint is not hex: %w", err)
	}
	return "android:apk-key-hash:" + base64.RawURLEncoding.EncodeToString(raw), nil
}

// NativeAppOrigins derives every extra WebAuthn origin this instance's listed
// apps sign in from.
//
// Apple needs nothing here: a platform assertion from an iOS app associated
// with the domain carries "https://<domain>", which the relying party already
// accepts. Android carries the apk-key-hash origin instead, so an app the
// admin listed cannot sign in until that origin is on the list — which is why
// this is called at startup and again after every add and remove.
//
// A malformed fingerprint is skipped rather than fatal: the write paths check
// the shape, and a row that somehow got past them must not be able to stop
// the server from booting.
func NativeAppOrigins(db *database.DB) ([]string, error) {
	rows, err := db.Query(`SELECT fingerprints FROM native_apps WHERE platform = 'android'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	seen := map[string]bool{}
	origins := []string{}
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			continue
		}
		var prints []string
		if json.Unmarshal([]byte(raw), &prints) != nil {
			continue
		}
		for _, p := range prints {
			origin, err := APKKeyHashOrigin(p)
			if err != nil || seen[origin] {
				continue
			}
			seen[origin] = true
			origins = append(origins, origin)
		}
	}
	return origins, rows.Err()
}
