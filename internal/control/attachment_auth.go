package control

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

// MintAttachmentGrant stores a presentation capability in tx. The trusted core
// caller MUST already retain the same run/root's previously acquired ServiceOwner,
// its live instance-generation hold and the runtime authority read guard. These
// remain held through commit and response queue acceptance/discard (design §4.1).
// Peer authentication and SQL scope are additional checks, never ownership proof.
// The returned token is provisional process memory: discard it on any later
// transaction/encoding failure; only Task12 may compose a committed public reply.
func MintAttachmentGrant(ctx context.Context, tx *sql.Tx, peer Peer, runID string, generation int64) (AttachmentGrant, error) {
	if err := attachmentAccess(peer, tx); err != nil {
		return AttachmentGrant{}, err
	}
	if generation <= 0 {
		return AttachmentGrant{}, attachmentFailure(CodeOwnershipUnresolved)
	}
	var source string
	err := tx.QueryRowContext(ctx, `SELECT source_repo FROM runs WHERE run_id=?`, runID).Scan(&source)
	if errors.Is(err, sql.ErrNoRows) {
		return AttachmentGrant{}, attachmentFailure(CodePermissionDenied)
	}
	if err != nil {
		return AttachmentGrant{}, attachmentFailure(CodePersistenceUnavailable)
	}
	repository, ok := attachmentRepository(source)
	if !ok {
		return AttachmentGrant{}, attachmentFailure(CodeOwnershipUnresolved)
	}
	// All three identifiers are core-minted. No pane, PID or caller label
	// participates in capability identity; only the bearer token grants access.
	grant := AttachmentGrant{
		AttachmentID: rand.Text(), ClientID: rand.Text(), Token: rand.Text(),
		RunID: runID, RepositoryID: repository, Generation: generation,
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO attachment_grants
		(attachment_id,token_sha256,client_id,run_id,repository_id,generation,revocation_revision,revoked)
		VALUES (?,?,?,?,?,?,0,0)`, grant.AttachmentID, tokenDigest(grant.Token), grant.ClientID,
		grant.RunID, grant.RepositoryID, grant.Generation)
	if err != nil {
		return AttachmentGrant{}, attachmentFailure(CodePersistenceUnavailable)
	}
	return grant, nil
}

// ValidateAttachmentGrant authenticates exact stored scope before classifying
// generation. It requires MintAttachmentGrant's actual retained owner/instance
// precondition and runtime authority read guard; supplying a generation alone
// proves nothing. It neither reconciles nor acquires execution ownership.
func ValidateAttachmentGrant(ctx context.Context, tx *sql.Tx, peer Peer, grant AttachmentGrant, generation int64) error {
	if err := attachmentAccess(peer, tx); err != nil {
		return err
	}
	if err := grant.Validate(); err != nil {
		return attachmentFailure(CodePermissionDenied)
	}
	var stored AttachmentGrant
	var digest, source string
	var revoked int
	err := tx.QueryRowContext(ctx, `SELECT a.client_id,a.run_id,a.repository_id,a.generation,a.token_sha256,a.revoked,r.source_repo
		FROM attachment_grants a JOIN runs r ON r.run_id=a.run_id
		WHERE a.attachment_id=? AND a.run_id=?`, grant.AttachmentID, grant.RunID).
		Scan(&stored.ClientID, &stored.RunID, &stored.RepositoryID, &stored.Generation, &digest, &revoked, &source)
	if errors.Is(err, sql.ErrNoRows) {
		return attachmentFailure(CodePermissionDenied)
	}
	if err != nil {
		return attachmentFailure(CodePersistenceUnavailable)
	}
	repository, ok := attachmentRepository(source)
	if !ok || revoked != 0 || stored.ClientID != grant.ClientID || stored.RunID != grant.RunID ||
		stored.RepositoryID != grant.RepositoryID || repository != grant.RepositoryID ||
		stored.Generation != grant.Generation || subtle.ConstantTimeCompare([]byte(digest), []byte(tokenDigest(grant.Token))) != 1 {
		return attachmentFailure(CodePermissionDenied)
	}
	// I03/I17 (v2 §§8/14): forged or revoked authority cannot probe epochs.
	if generation <= 0 || stored.Generation != generation {
		return attachmentFailure(CodeRevisionConflict)
	}
	return nil
}

// RevokeAttachmentGrant changes only attachment authority. The trusted caller
// holds the same retained owner and instance as MintAttachmentGrant, but uses
// the runtime authority WRITE guard, serialising revocation with read handoff.
// The caller commits or rolls back this transaction before releasing that guard.
func RevokeAttachmentGrant(ctx context.Context, tx *sql.Tx, peer Peer, grant AttachmentGrant, generation int64) error {
	if err := ValidateAttachmentGrant(ctx, tx, peer, grant, generation); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE attachment_grants SET revoked=1,revocation_revision=revocation_revision+1
		WHERE attachment_id=? AND revoked=0`, grant.AttachmentID)
	if err != nil {
		return attachmentFailure(CodePersistenceUnavailable)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return attachmentFailure(CodePersistenceUnavailable)
	}
	if count != 1 {
		return attachmentFailure(CodePermissionDenied)
	}
	return nil
}

func attachmentAccess(peer Peer, tx *sql.Tx) error {
	// Authentication must precede every transaction access, including failures.
	if !peer.SameUser {
		return attachmentFailure(CodePermissionDenied)
	}
	if tx == nil {
		return attachmentFailure(CodePersistenceUnavailable)
	}
	return nil
}

func attachmentFailure(code Code) error {
	return newError(code, "attachment unavailable")
}

// attachmentRepository names exact recorded canonical scope; it makes no new
// filesystem identity claim. Root/database identity is the retained owner's job.
func attachmentRepository(source string) (string, bool) {
	if source == "" || !utf8.ValidString(source) || !filepath.IsAbs(source) || filepath.Clean(source) != source || strings.ContainsFunc(source, unicode.IsControl) {
		return "", false
	}
	bytes := []byte("mythhelm.attachment.repository.v1\x00")
	bytes = binary.BigEndian.AppendUint64(bytes, uint64(len(source)))
	bytes = append(bytes, source...)
	sum := sha256.Sum256(bytes)
	return hex.EncodeToString(sum[:]), true
}
