package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"github.com/turbokast/mythhelm/internal/control"
)

func publicAttachmentDenied() error {
	return &control.Error{Code: control.CodePermissionDenied, Message: "attachment unavailable"}
}
func publicAttachmentFailure() error {
	return &control.Error{Code: control.CodePersistenceUnavailable, Message: "attachment unavailable"}
}
func publicAttachmentAuthError(err error) error {
	if errors.Is(err, &control.Error{Code: control.CodeRevisionConflict}) {
		return &control.Error{Code: control.CodeRevisionConflict, Message: "attachment unavailable"}
	}
	return publicAttachmentDenied()
}

// attachmentHandler is reached only through the authenticated transport table.
// A real retained scope precedes its single transaction; it never acquires,
// reconciles or dispatches execution. The scope passes to the owned queue.
func (r *runtime) attachmentHandler(ctx context.Context, peer control.Peer, in control.Intent) (*control.AttachmentResponse, error) {
	if !peer.SameUser {
		return nil, publicAttachmentDenied()
	}
	if err := in.Validate(); err != nil {
		return nil, &control.Error{Code: control.CodeInvalidContract, Message: "invalid attachment contract"}
	}
	var runID string
	var grant control.AttachmentGrant
	if in.Method == "attachment.open" {
		params, err := control.DecodeParams[attachmentOpenInput](in.Params)
		if err != nil || params.RunID != in.Object {
			return nil, publicAttachmentInvalid()
		}
		runID = params.RunID
	} else {
		params, err := control.DecodeParams[attachmentGrantInput](in.Params)
		if err != nil {
			return nil, publicAttachmentInvalid()
		}
		grant = params.Grant
		runID = grant.RunID
		if in.Object != runID || in.Generation == nil || *in.Generation != grant.Generation || in.CapabilityToken != grant.Token {
			return nil, publicAttachmentInvalid()
		}
	}
	scope, err := r.retainAttachmentScope(ctx, runID, in.Method == "attachment.close")
	if err != nil {
		return nil, publicAttachmentDenied()
	}
	transferred := false
	defer func() {
		if !transferred {
			scope.release()
		}
	}()
	generation := r.lease.Generation()
	var body []byte
	var revision uint64
	authorized := false
	err = r.journal.Transact(ctx, func(tx *sql.Tx) error {
		switch in.Method {
		case "attachment.open":
			var err error
			grant, err = control.MintAttachmentGrant(ctx, tx, peer, runID, generation)
			if err != nil {
				return publicAttachmentDenied()
			}
			authorized = true
		case "attachment.read":
			if err := control.ValidateAttachmentGrant(ctx, tx, peer, grant, generation); err != nil {
				return publicAttachmentAuthError(err)
			}
			authorized = true
		case "attachment.close":
			if err := control.RevokeAttachmentGrant(ctx, tx, peer, grant, generation); err != nil {
				return publicAttachmentAuthError(err)
			}
			authorized = true
			body = []byte(`{}`)
			return nil
		default:
			return publicAttachmentDenied()
		}
		projection, err := control.SelectAttachmentProjection(ctx, tx, grant, r.journal.StateDir())
		if err != nil {
			return publicAttachmentFailure()
		}
		revision = projection.RunRevision
		if in.Method == "attachment.open" {
			body, err = json.Marshal(control.OpenAttachment{Grant: grant, Projection: projection})
		} else {
			body, err = json.Marshal(projection)
		}
		if err != nil {
			return publicAttachmentFailure()
		}
		// Encoding/bounds failure must roll back the initial mint and cache too.
		if len(body) > control.AttachmentFrameBytes-1024 {
			return publicAttachmentFailure()
		}
		if err := ctx.Err(); err != nil {
			return publicAttachmentFailure()
		}
		return nil
	})
	if err != nil {
		if ce, ok := errors.AsType[*control.Error](err); ok {
			return nil, ce
		}
		if authorized {
			return nil, publicAttachmentFailure()
		}
		return nil, publicAttachmentDenied()
	}
	if err := ctx.Err(); err != nil {
		return nil, publicAttachmentFailure()
	}
	response, err := control.NewAttachmentResponse(control.Result{Body: body, Revision: int64(revision)}, scope.release)
	if err != nil {
		return nil, publicAttachmentFailure()
	}
	transferred = true
	return response, nil
}

func (r *runtime) registerAttachments() error {
	for _, method := range []string{"attachment.open", "attachment.read", "attachment.close"} {
		if err := r.server.RegisterAttachment(method, r.attachmentHandler); err != nil {
			return err
		}
	}
	return nil
}

func publicAttachmentInvalid() error {
	return &control.Error{Code: control.CodeInvalidContract, Message: "invalid attachment contract"}
}

type attachmentOpenInput struct {
	RunID string `json:"run_id"`
}

func (p attachmentOpenInput) Validate() error {
	if p.RunID == "" || strings.TrimSpace(p.RunID) != p.RunID {
		return publicAttachmentInvalid()
	}
	return nil
}

type attachmentGrantInput struct {
	Grant control.AttachmentGrant `json:"grant"`
}

func (p attachmentGrantInput) Validate() error { return p.Grant.Validate() }
