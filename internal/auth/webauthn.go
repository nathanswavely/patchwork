package auth

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"

	"github.com/patchwork-toolkit/patchwork/internal/config"
	"github.com/patchwork-toolkit/patchwork/internal/database"
	"github.com/patchwork-toolkit/patchwork/internal/model"
)

// WebAuthnService manages WebAuthn ceremonies.
type WebAuthnService struct {
	wa       *webauthn.WebAuthn
	db       *database.DB
	sessions *sessionStore
}

// maxWebAuthnSessions bounds the in-memory challenge store. Login challenges
// are created by unauthenticated requests, so without a cap the map grows
// with traffic and only sheds entries on the 5-minute TTL. At this size the
// store costs a few MB, which is affordable on the 2GB target host, and it is
// far above the number of passkey ceremonies a real community has in flight.
const maxWebAuthnSessions = 10000

// sessionStore is an in-memory store for WebAuthn session data with TTL and a
// hard size cap.
type sessionStore struct {
	mu      sync.Mutex
	data    map[string]*sessionEntry
	maxSize int
}

type sessionEntry struct {
	sessionData *webauthn.SessionData
	expiresAt   time.Time
}

func newSessionStore() *sessionStore {
	s := &sessionStore{
		data:    make(map[string]*sessionEntry),
		maxSize: maxWebAuthnSessions,
	}
	// Background cleanup every minute.
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			s.cleanup()
		}
	}()
	return s
}

func (s *sessionStore) Set(key string, sd *webauthn.SessionData) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Only enforce the cap when inserting a new key; overwriting an existing
	// one cannot grow the map.
	if _, exists := s.data[key]; !exists {
		s.evictLocked()
	}

	s.data[key] = &sessionEntry{
		sessionData: sd,
		expiresAt:   time.Now().Add(5 * time.Minute),
	}
}

// evictLocked makes room for one new entry. It first drops anything expired,
// and if the store is still full, evicts the entry closest to expiry — the
// oldest ceremony, and so the one least likely to still be completed. Callers
// must hold s.mu.
func (s *sessionStore) evictLocked() {
	if len(s.data) < s.maxSize {
		return
	}

	now := time.Now()
	for k, v := range s.data {
		if now.After(v.expiresAt) {
			delete(s.data, k)
		}
	}
	if len(s.data) < s.maxSize {
		return
	}

	var oldestKey string
	var oldestAt time.Time
	for k, v := range s.data {
		if oldestKey == "" || v.expiresAt.Before(oldestAt) {
			oldestKey, oldestAt = k, v.expiresAt
		}
	}
	if oldestKey != "" {
		delete(s.data, oldestKey)
	}
}

func (s *sessionStore) Get(key string) (*webauthn.SessionData, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.data[key]
	if !ok || time.Now().After(entry.expiresAt) {
		delete(s.data, key)
		return nil, false
	}
	return entry.sessionData, true
}

func (s *sessionStore) Delete(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, key)
}

func (s *sessionStore) cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for k, v := range s.data {
		if now.After(v.expiresAt) {
			delete(s.data, k)
		}
	}
}

// NewWebAuthnService creates a configured WebAuthn service.
func NewWebAuthnService(db *database.DB, cfg *config.Config) (*WebAuthnService, error) {
	rpID := cfg.Instance.Domain
	rpOrigins := []string{"https://" + cfg.Instance.Domain}

	wa, err := webauthn.New(&webauthn.Config{
		RPDisplayName: cfg.Instance.Name,
		RPID:          rpID,
		RPOrigins:     rpOrigins,
	})
	if err != nil {
		return nil, fmt.Errorf("init webauthn: %w", err)
	}

	return &WebAuthnService{
		wa:       wa,
		db:       db,
		sessions: newSessionStore(),
	}, nil
}

// WebAuthnUser adapts a model.User + credentials for the webauthn.User interface.
type WebAuthnUser struct {
	User        *model.User
	Credentials []webauthn.Credential
}

func (u *WebAuthnUser) WebAuthnID() []byte                         { return []byte(u.User.ID) }
func (u *WebAuthnUser) WebAuthnName() string                       { return u.User.Username }
func (u *WebAuthnUser) WebAuthnDisplayName() string                { return u.User.DisplayName }
func (u *WebAuthnUser) WebAuthnCredentials() []webauthn.Credential { return u.Credentials }

// DefaultCredentialName is used when someone enrolls a passkey without naming
// it, and is the SQL default on credentials.name.
const DefaultCredentialName = "Passkey"

// maxCredentialNameLen caps a passkey nickname. Long enough for "Nathan's
// work laptop (Touch ID)", short enough that the list stays readable.
const maxCredentialNameLen = 64

// SanitizeCredentialName cleans a person-supplied passkey name: trims
// surrounding whitespace, drops control characters (which would otherwise let
// a name break the credential list's layout or smuggle newlines into logs),
// and caps the length in runes rather than bytes so multi-byte names are not
// cut mid-character. Anything that reduces to nothing falls back to the
// default — an unnamed passkey is still a usable passkey.
func SanitizeCredentialName(name string) string {
	cleaned := strings.Map(func(r rune) rune {
		if r == '\t' || r == '\n' || r == '\r' {
			return ' '
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, name)

	cleaned = strings.TrimSpace(cleaned)
	if cleaned == "" {
		return DefaultCredentialName
	}

	if runes := []rune(cleaned); len(runes) > maxCredentialNameLen {
		cleaned = strings.TrimSpace(string(runes[:maxCredentialNameLen]))
		if cleaned == "" {
			return DefaultCredentialName
		}
	}
	return cleaned
}

// encodeTransports serializes the authenticator's transport hints for storage.
// Empty stays NULL rather than "[]" so "the authenticator told us nothing" and
// "we never asked" are not conflated.
func encodeTransports(transports []protocol.AuthenticatorTransport) any {
	if len(transports) == 0 {
		return nil
	}
	b, err := json.Marshal(transports)
	if err != nil {
		return nil
	}
	return string(b)
}

// SaveCredential writes a freshly created WebAuthn credential record. It
// persists the full record — including the backup flags and transports that
// used to be dropped on the floor (issue #50) — and returns the new row's ID.
func SaveCredential(db *database.DB, userID, name string, cred *webauthn.Credential) (string, error) {
	credRowID := NewUUIDv7()
	_, err := db.Exec(
		`INSERT INTO credentials
		    (id, user_id, credential_id, public_key, attestation_type, aaguid, sign_count, name, backup_eligible, backup_state, transports)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		credRowID, userID, cred.ID, cred.PublicKey, cred.AttestationType,
		cred.Authenticator.AAGUID, cred.Authenticator.SignCount,
		SanitizeCredentialName(name),
		cred.Flags.BackupEligible, cred.Flags.BackupState,
		encodeTransports(cred.Transport),
	)
	if err != nil {
		return "", err
	}
	return credRowID, nil
}

// LoadCredentials fetches a user's WebAuthn credentials, restoring every field
// the library needs to validate an assertion.
//
// backup_eligible and backup_state are nullable: NULL marks a row enrolled
// before Patchwork tracked flags. Those load as false, exactly as they did
// before — deliberately. There is no healing from the assertion here, because
// adopting whatever flags a login presents would mean trusting the assertion
// to describe the credential it is authenticating against, which is precisely
// the check the stored flag exists to perform.
func LoadCredentials(db *database.DB, userID string) ([]webauthn.Credential, error) {
	rows, err := db.Query(
		`SELECT credential_id, public_key, attestation_type, aaguid, sign_count,
		        backup_eligible, backup_state, transports
		   FROM credentials WHERE user_id = ?`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var creds []webauthn.Credential
	for rows.Next() {
		var c webauthn.Credential
		var aaguid []byte
		var backupEligible, backupState sql.NullBool
		var transports sql.NullString

		err := rows.Scan(
			&c.ID, &c.PublicKey, &c.AttestationType, &aaguid, &c.Authenticator.SignCount,
			&backupEligible, &backupState, &transports,
		)
		if err != nil {
			return nil, err
		}

		// Assigned, not copied into. AAGUID is a []byte slice, not a fixed
		// array — the old copy(dst[:], src) copied into a nil slice and threw
		// the AAGUID away on every load. It is what identifies the
		// authenticator model to MDS metadata validation.
		c.Authenticator.AAGUID = aaguid
		c.Flags.BackupEligible = backupEligible.Bool
		c.Flags.BackupState = backupState.Bool
		if transports.Valid && transports.String != "" {
			json.Unmarshal([]byte(transports.String), &c.Transport)
		}

		creds = append(creds, c)
	}
	return creds, rows.Err()
}

// loadCredentials fetches WebAuthn credentials for a user from the DB.
func (s *WebAuthnService) loadCredentials(userID string) ([]webauthn.Credential, error) {
	return LoadCredentials(s.db, userID)
}

// buildWebAuthnUser creates a WebAuthnUser from a model.User.
func (s *WebAuthnService) buildWebAuthnUser(user *model.User) (*WebAuthnUser, error) {
	creds, err := s.loadCredentials(user.ID)
	if err != nil {
		return nil, err
	}
	return &WebAuthnUser{User: user, Credentials: creds}, nil
}

// BeginRegistration starts a WebAuthn registration ceremony.
func (s *WebAuthnService) BeginRegistration(user *model.User) ([]byte, error) {
	waUser, err := s.buildWebAuthnUser(user)
	if err != nil {
		return nil, err
	}

	options, session, err := s.wa.BeginRegistration(waUser)
	if err != nil {
		return nil, fmt.Errorf("begin registration: %w", err)
	}

	s.sessions.Set("reg:"+user.ID, session)

	optJSON, err := json.Marshal(options)
	if err != nil {
		return nil, err
	}
	return optJSON, nil
}

// FinishRegistration completes registration and stores the credential.
// The name is the person's own label for this passkey ("iPhone", "YubiKey on
// my keys"); empty falls back to the default.
func (s *WebAuthnService) FinishRegistration(user *model.User, response *protocol.ParsedCredentialCreationData, name string) (*model.Credential, error) {
	waUser, err := s.buildWebAuthnUser(user)
	if err != nil {
		return nil, err
	}

	sessionData, ok := s.sessions.Get("reg:" + user.ID)
	if !ok {
		return nil, fmt.Errorf("no registration session found")
	}
	s.sessions.Delete("reg:" + user.ID)

	cred, err := s.wa.CreateCredential(waUser, *sessionData, response)
	if err != nil {
		return nil, fmt.Errorf("create credential: %w", err)
	}

	// Store in DB.
	credID, err := SaveCredential(s.db, user.ID, name, cred)
	if err != nil {
		return nil, fmt.Errorf("store credential: %w", err)
	}

	return &model.Credential{
		ID:        credID,
		UserID:    user.ID,
		Name:      SanitizeCredentialName(name),
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}, nil
}

// ErrNoCredentials is returned when a step-up ceremony is requested by
// someone with no passkey enrolled. Callers surface it as a distinct,
// machine-readable condition so the UI can offer enrollment rather than
// reporting a generic failure.
var ErrNoCredentials = errors.New("no passkey enrolled")

// BeginStepUp starts a WebAuthn assertion for an already-authenticated user
// (docs/adr/017). Unlike BeginLogin this is not discoverable: the challenge is
// scoped to the credentials of the person holding the session, so the only
// thing it can prove is that *they* are present at their authenticator.
func (s *WebAuthnService) BeginStepUp(user *model.User) ([]byte, error) {
	waUser, err := s.buildWebAuthnUser(user)
	if err != nil {
		return nil, err
	}
	if len(waUser.Credentials) == 0 {
		return nil, ErrNoCredentials
	}

	options, session, err := s.wa.BeginLogin(waUser)
	if err != nil {
		return nil, fmt.Errorf("begin step-up: %w", err)
	}

	s.sessions.Set("sudo:"+user.ID, session)

	optJSON, err := json.Marshal(options)
	if err != nil {
		return nil, err
	}
	return optJSON, nil
}

// FinishStepUp verifies the assertion against the session holder's own
// credentials. It returns an error if the assertion was produced by anyone
// else's authenticator, so a step-up window can only ever be opened by the
// person who owns the session.
func (s *WebAuthnService) FinishStepUp(user *model.User, response *protocol.ParsedCredentialAssertionData) error {
	waUser, err := s.buildWebAuthnUser(user)
	if err != nil {
		return err
	}
	if len(waUser.Credentials) == 0 {
		return ErrNoCredentials
	}

	sessionData, ok := s.sessions.Get("sudo:" + user.ID)
	if !ok {
		return fmt.Errorf("no step-up challenge found — it may have expired")
	}
	// Single use: a challenge that verified once cannot be replayed into a
	// second window.
	s.sessions.Delete("sudo:" + user.ID)

	cred, err := s.wa.ValidateLogin(waUser, *sessionData, response)
	if err != nil {
		return fmt.Errorf("validate step-up: %w", err)
	}

	s.db.Exec(`UPDATE credentials SET sign_count = ? WHERE credential_id = ?`, cred.Authenticator.SignCount, cred.ID)
	return nil
}

// BeginLogin starts a WebAuthn login ceremony for discoverable credentials.
func (s *WebAuthnService) BeginLogin() ([]byte, error) {
	options, session, err := s.wa.BeginDiscoverableLogin()
	if err != nil {
		return nil, fmt.Errorf("begin login: %w", err)
	}

	// Use challenge as session key.
	key := fmt.Sprintf("login:%x", session.Challenge)
	s.sessions.Set(key, session)

	optJSON, err := json.Marshal(options)
	if err != nil {
		return nil, err
	}
	return optJSON, nil
}

// FinishLogin completes login and returns the user.
func (s *WebAuthnService) FinishLogin(response *protocol.ParsedCredentialAssertionData) (*model.User, error) {
	// Find the session by challenge.
	key := fmt.Sprintf("login:%x", response.Response.CollectedClientData.Challenge)
	sessionData, ok := s.sessions.Get(key)
	if !ok {
		return nil, fmt.Errorf("no login session found")
	}
	s.sessions.Delete(key)

	// Discoverable login handler: look up user by credential.
	var foundUser *WebAuthnUser

	handler := func(rawID, userHandle []byte) (webauthn.User, error) {
		// userHandle is the user ID we set during registration.
		userID := string(userHandle)
		var user model.User
		err := s.db.QueryRow(
			`SELECT id, COALESCE(email,''), username, display_name, bio, avatar_url, role, created_at, updated_at FROM users WHERE id = ?`,
			userID,
		).Scan(&user.ID, &user.Email, &user.Username, &user.DisplayName, &user.Bio, &user.AvatarURL, &user.Role, &user.CreatedAt, &user.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("user not found: %w", err)
		}

		waUser, err := s.buildWebAuthnUser(&user)
		if err != nil {
			return nil, err
		}
		foundUser = waUser
		return waUser, nil
	}

	cred, err := s.wa.ValidateDiscoverableLogin(handler, *sessionData, response)
	if err != nil {
		return nil, fmt.Errorf("validate login: %w", err)
	}

	// Persist what this assertion taught us: the advanced sign counter, and the
	// refreshed backup state the library wrote back onto the credential.
	//
	// backup_state is only touched on rows that already carry flags. A row with
	// NULL flags predates issue #50's fix, and writing a state onto it would be
	// adopting the assertion's word for a credential we never recorded — the
	// heal we deliberately do not do. Those rows stay NULL and keep failing
	// until the person re-enrolls.
	//
	// A failure here is not fatal to the login — the person authenticated
	// correctly — but a sign counter that silently stops advancing defeats
	// clone detection, so it must not pass unnoticed.
	if _, err := s.db.Exec(
		`UPDATE credentials
		    SET sign_count = ?,
		        backup_state = CASE WHEN backup_eligible IS NULL THEN backup_state ELSE ? END
		  WHERE credential_id = ?`,
		cred.Authenticator.SignCount, cred.Flags.BackupState, cred.ID,
	); err != nil {
		log.Printf("webauthn: could not update credential after login: %v", err)
	}

	if foundUser == nil {
		return nil, fmt.Errorf("user not resolved during login")
	}

	return foundUser.User, nil
}
