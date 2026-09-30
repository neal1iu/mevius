package catalog

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"time"
)

type OperationInput struct {
	TypeID     string   `json:"resource_type_id"`
	ActionID   string   `json:"action_id"`
	TargetKind string   `json:"target_kind"`
	TargetID   string   `json:"target_id"`
	AccessID   string   `json:"access_id,omitempty"`
	Input      Document `json:"input"`
}
type Operation struct {
	ResultResourceID string    `json:"result_resource_id,omitempty"`
	ID               string    `json:"id"`
	ActionID         string    `json:"action_id"`
	TypeID           string    `json:"resource_type_id"`
	TargetKind       string    `json:"target_kind"`
	TargetID         string    `json:"target_id"`
	AccessID         string    `json:"access_id"`
	ConnectionID     string    `json:"connection_id"`
	Revision         int       `json:"credential_revision"`
	Snapshot         Document  `json:"snapshot"`
	Status           string    `json:"status"`
	RemoteID         string    `json:"remote_operation_id,omitempty"`
	Result           Document  `json:"result"`
	ErrorCode        string    `json:"error_code,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

func scanOperation(row interface{ Scan(...any) error }) (*Operation, string, error) {
	v := &Operation{}
	var key, hash, snap, result string
	var c, u int64
	e := row.Scan(&v.ID, &key, &hash, &v.ActionID, &v.TypeID, &v.TargetKind, &v.TargetID, &v.AccessID, &v.ConnectionID, &v.Revision, &v.Snapshot.Version, &snap, &v.Status, &v.RemoteID, &v.Result.Version, &result, &v.ErrorCode, &v.ResultResourceID, &c, &u)
	v.Snapshot.Data = JSON(snap)
	v.Result.Data = JSON(result)
	v.CreatedAt = timestamp(c)
	v.UpdatedAt = timestamp(u)
	if e == nil && (!json.Valid(v.Snapshot.Data) || !json.Valid(v.Result.Data)) {
		return nil, "", ErrInvalid
	}
	return v, hash, dbError(e)
}
func (s *Service) Operation(ctx context.Context, id string) (*Operation, error) {
	v, _, e := scanOperation(s.DB.QueryRowContext(ctx, `SELECT * FROM operation_request WHERE id=?`, id))
	if e == nil {
		e = s.validateOperation(v)
	}
	return v, e
}
func (s *Service) validateOperation(v *Operation) error {
	if v.Snapshot.Version != 1 || v.Result.Version < 1 {
		return ErrInvalid
	}
	if t := s.Registry.types[v.TypeID]; t != nil {
		version := 0
		for _, a := range t.Descriptor.Actions {
			if a.ID == v.ActionID {
				version = a.Output.Version
			}
		}
		if version == 0 {
			return nil
		}
		if version != v.Result.Version {
			return ErrInvalid
		}
		if v.Status == "succeeded" {
			return s.Registry.Validate(v.TypeID, "action-output:"+v.ActionID, v.Result)
		}
	}
	return nil
}
func (s *Service) Operations(ctx context.Context) ([]Operation, error) {
	rows, e := s.DB.QueryContext(ctx, `SELECT * FROM operation_request ORDER BY created_at DESC,id LIMIT 200`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Operation{}
	for rows.Next() {
		v, _, e := scanOperation(rows)
		if e != nil {
			return nil, e
		}
		if e = s.validateOperation(v); e != nil {
			return nil, e
		}
		out = append(out, *v)
	}
	return out, rows.Err()
}
func (s *Service) Submit(ctx context.Context, key string, input OperationInput) (*Operation, error) {
	if key == "" || len(key) > 200 || !json.Valid(input.Input.Data) {
		return nil, ErrInvalid
	}
	var canonical any
	dec := json.NewDecoder(bytes.NewReader(input.Input.Data))
	dec.UseNumber()
	if e := dec.Decode(&canonical); e != nil {
		return nil, ErrInvalid
	}
	input.Input.Data = jsonBytes(canonical)
	mac := hmac.New(sha256.New, s.key[:])
	mac.Write(jsonBytes(input))
	fingerprint := hex.EncodeToString(mac.Sum(nil))
	existing, hash, e := scanOperation(s.DB.QueryRowContext(ctx, `SELECT * FROM operation_request WHERE idempotency_key=?`, key))
	if e == nil {
		if hash != fingerprint {
			return nil, ErrConflict
		}
		return existing, nil
	}
	if e != ErrNotFound {
		return nil, e
	}
	t := s.Registry.types[input.TypeID]
	if t == nil || t.Handler == nil {
		return nil, ErrForbidden
	}
	var action *ActionDescriptor
	for _, a := range t.Descriptor.Actions {
		if a.ID == input.ActionID {
			v := a
			action = &v
		}
	}
	if action == nil || action.Target != input.TargetKind {
		return nil, ErrInvalid
	}
	if e = s.Registry.Validate(input.TypeID, "action:"+action.ID, input.Input); e != nil {
		return nil, e
	}
	var r *Resource
	var a *Access
	var c *AccessContext
	var capabilities Capabilities
	if input.TargetKind == "scope" {
		if input.AccessID != "" {
			return nil, ErrInvalid
		}
		c, e = s.Context(ctx, input.TargetID)
		if e == nil {
			e = decode(string(c.Binding.Context.Data), &capabilities)
		}
	} else {
		r, e = s.Resource(ctx, input.TargetID)
		if e == nil && r.TypeID != input.TypeID {
			return nil, ErrInvalid
		}
		if e == nil {
			a, c, e = s.SelectAccess(ctx, r, input.AccessID)
			if e == nil {
				if a.ErrorCode != "" {
					return nil, ErrForbidden
				}
				capabilities = a.Capabilities
			}
		}
	}
	if e != nil {
		return nil, e
	}
	if c.Instance.ProviderID != t.Descriptor.ProviderID {
		return nil, ErrInvalid
	}
	permission := capabilities[action.Capability].Availability
	if permission != "available" && !(permission == "unknown" && action.Authorization == "remote" && action.Effect != "delete") {
		return nil, ErrForbidden
	}
	now := time.Now().UTC()
	aid := c.Binding.ID
	if a != nil {
		aid = a.ID
	}
	snapshot := map[string]any{"permission": permission, "capability": action.Capability, "authorization_policy": action.Authorization, "provider_instance_id": c.Instance.ID, "connection_scope_id": c.Binding.ID, "scope_id": c.Scope.ID, "credential_revision": c.Connection.Revision}
	if r != nil {
		snapshot["resource_id"] = r.ID
		snapshot["identity_key"] = r.IdentityKey
		snapshot["resource_type_id"] = r.TypeID
	}
	op := &Operation{ID: uuid.NewString(), ActionID: input.ActionID, TypeID: input.TypeID, TargetKind: input.TargetKind, TargetID: input.TargetID, AccessID: aid, ConnectionID: c.Connection.ID, Revision: c.Connection.Revision, Snapshot: Document{1, jsonBytes(snapshot)}, Status: "in_progress", Result: Document{1, JSON(`{}`)}, CreatedAt: now, UpdatedAt: now}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback()
	var revision int
	var state string
	if e = tx.QueryRowContext(ctx, `SELECT p.credential_revision,b.validation_state FROM provider_connection p JOIN connection_scope b ON b.connection_id=p.id WHERE b.id=?`, c.Binding.ID).Scan(&revision, &state); e != nil {
		return nil, dbError(e)
	}
	if revision != c.Connection.Revision || state != "valid" {
		return nil, ErrConflict
	}
	if r != nil {
		var active int
		if e = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM operation_request WHERE target_kind='resource' AND target_id=? AND status='in_progress'`, r.ID).Scan(&active); e != nil {
			return nil, e
		}
		if active > 0 {
			return nil, ErrConflict
		}
		if action.Effect == "delete" {
			if t.Descriptor.Identity.Natural {
				return nil, fmt.Errorf("%w: natural identity cannot prove the current remote creation", ErrForbidden)
			}
			current, e := scanResource(tx.QueryRowContext(ctx, `SELECT * FROM resource_instance WHERE id=?`, r.ID))
			if e != nil {
				return nil, e
			}
			if e = deletionCheck(ctx, tx, current, true); e != nil {
				return nil, e
			}
		}
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO operation_request VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, op.ID, key, fingerprint, op.ActionID, op.TypeID, op.TargetKind, op.TargetID, op.AccessID, op.ConnectionID, op.Revision, 1, string(op.Snapshot.Data), op.Status, "", action.Output.Version, "{}", "", "", millis(now), millis(now))
	if e != nil {
		tx.Rollback()
		existing, hash, lookupErr := scanOperation(s.DB.QueryRowContext(ctx, `SELECT * FROM operation_request WHERE idempotency_key=?`, key))
		if lookupErr == nil && hash == fingerprint {
			return existing, nil
		}
		return nil, ErrConflict
	}
	if e = tx.Commit(); e != nil {
		return nil, e
	}
	// Use the request deadline. Cancellation after submission is an ambiguous remote outcome.
	var result ActionResult
	var remoteErr error
	if r != nil {
		live, inspectErr := t.Handler.Inspect(ctx, *c, r.Locator.Data)
		if inspectErr != nil {
			result.FailureCode = "preflight_failed"
		} else if validationErr := s.validateObservation(t, c, live); validationErr != nil {
			result.FailureCode = "preflight_failed"
		} else {
			key, keyErr := IdentityKey(live.IdentityParts)
			if keyErr != nil || key != r.IdentityKey {
				result.FailureCode = "identity_changed"
			} else {
				permission := live.Capabilities[action.Capability].Availability
				if permission == "unavailable" || permission == "" || (action.Effect == "delete" && permission != "available") {
					result.FailureCode = "denied"
				} else {
					current := *r
					current.Locator = Document{t.Descriptor.Locator.Version, live.Locator}
					r = &current
					currentAccess := *a
					currentAccess.Observation = Document{t.Descriptor.Observation.Version, live.Public}
					currentAccess.Capabilities = live.Capabilities
					a = &currentAccess
				}
			}
		}
	}
	if result.FailureCode == "" {
		result, remoteErr = t.Actions[action.ID](ctx, *c, r, a, input.Input.Data)
	}
	status := "succeeded"
	errorCode := ""
	public := result.Public
	if len(public) == 0 {
		public = JSON(`{}`)
	}
	if result.FailureCode != "" && member([]string{"denied", "not_found", "rate_limited", "invalid_input", "identity_changed", "preflight_failed"}, result.FailureCode) {
		status = "failed"
		errorCode = result.FailureCode
		public = JSON(`{}`)
	} else if remoteErr != nil || result.Unknown {
		status = "unknown"
		errorCode = "remote_outcome_unknown"
		public = JSON(`{}`)
	} else if e = s.Registry.Validate(input.TypeID, "action-output:"+action.ID, Document{action.Output.Version, public}); e != nil {
		status = "unknown"
		errorCode = "invalid_public_result"
		public = JSON(`{}`)
	}
	if status == "succeeded" && action.Effect == "create" {
		if result.Observation == nil {
			status = "unknown"
			errorCode = "missing_creation_evidence"
		} else {
			created, persistErr := s.persist(ctx, c, t, *result.Observation, "mevius")
			if persistErr != nil {
				status = "unknown"
				errorCode = "catalog_commit_failed"
			} else {
				op.ResultResourceID = created.ID
			}
		}
	}
	// A fresh context persists the journal even when the caller disconnects.
	journalCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	finishTx, e := s.DB.BeginTx(journalCtx, nil)
	if e != nil {
		return op, e
	}
	defer finishTx.Rollback()
	if status == "succeeded" && action.Effect == "delete" {
		if _, e = finishTx.ExecContext(journalCtx, `DELETE FROM resource_instance WHERE id=?`, r.ID); e != nil {
			status = "unknown"
			errorCode = "catalog_commit_failed"
		}
	}
	_, e = finishTx.ExecContext(journalCtx, `UPDATE operation_request SET status=?,remote_operation_id=?,result_version=?,result_json=?,error_code=?,result_resource_id=?,updated_at=? WHERE id=?`, status, result.RemoteOperationID, action.Output.Version, string(public), errorCode, op.ResultResourceID, time.Now().UnixMilli(), op.ID)
	if e != nil {
		return op, e
	}
	if e = finishTx.Commit(); e != nil {
		return op, e
	}
	return s.Operation(journalCtx, op.ID)
}

// RecoverInterrupted is run once, before serving traffic. It never repeats remote writes.
func (s *Service) RecoverInterrupted(ctx context.Context) error {
	_, e := s.DB.ExecContext(ctx, `UPDATE operation_request SET status='unknown',error_code='process_interrupted',updated_at=? WHERE status='in_progress'`, time.Now().UnixMilli())
	return e
}

func (s *Service) CheckOperation(ctx context.Context, id string) (*Operation, error) {
	op, e := s.Operation(ctx, id)
	if e != nil {
		return nil, e
	}
	if op.Status == "in_progress" || op.RemoteID == "" {
		return nil, ErrConflict
	}
	t := s.Registry.types[op.TypeID]
	if t == nil || t.Checkers[op.ActionID] == nil {
		return nil, ErrForbidden
	}
	var snapshot struct {
		BindingID string `json:"connection_scope_id"`
	}
	if e = json.Unmarshal(op.Snapshot.Data, &snapshot); e != nil {
		return nil, ErrInvalid
	}
	c, e := s.Context(ctx, snapshot.BindingID)
	if e != nil {
		return nil, e
	}
	if c.Connection.ID != op.ConnectionID || c.Connection.Revision != op.Revision {
		return nil, ErrConflict
	}
	result, e := t.Checkers[op.ActionID](ctx, *c, *op)
	if e != nil {
		return nil, safeError(e)
	}
	if result.Unknown || len(result.Public) == 0 || result.RemoteOperationID != op.RemoteID {
		return nil, ErrUnknown
	}
	version := 0
	for _, a := range t.Descriptor.Actions {
		if a.ID == op.ActionID {
			version = a.Output.Version
		}
	}
	if e = s.Registry.Validate(op.TypeID, "action-output:"+op.ActionID, Document{version, result.Public}); e != nil {
		return nil, e
	}
	_, e = s.DB.ExecContext(ctx, `UPDATE operation_request SET result_version=?,result_json=?,status='succeeded',error_code='',updated_at=? WHERE id=? AND status<>'in_progress'`, version, string(result.Public), time.Now().UnixMilli(), id)
	if e != nil {
		return nil, e
	}
	return s.Operation(ctx, id)
}
