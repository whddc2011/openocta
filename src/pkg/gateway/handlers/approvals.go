package handlers

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cexll/agentsdk-go/pkg/security"
	"github.com/openocta/openocta/pkg/config"
	"github.com/openocta/openocta/pkg/gateway/protocol"
	"github.com/openocta/openocta/pkg/paths"
	octasecurity "github.com/openocta/openocta/pkg/security"
)

type approvalSnapshot struct {
	Records   []approvalRecord     `json:"records"`
	Whitelist map[string]time.Time `json:"whitelist"`
}

// approvalRecord mirrors agentsdk-go security.ApprovalRecord JSON fields.
type approvalRecord struct {
	ID           string     `json:"id"`
	SessionID    string     `json:"session_id"`
	Command      string     `json:"command"`
	Paths        []string   `json:"paths"`
	State        string     `json:"state"` // pending/approved/denied
	RequestedAt  time.Time  `json:"requested_at"`
	ApprovedAt   *time.Time `json:"approved_at,omitempty"`
	Approver     string     `json:"approver,omitempty"`
	Reason       string     `json:"reason,omitempty"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
	AutoApproved bool       `json:"auto_approved"`
}

func resolveApprovalStoreFile(cfg *config.OpenOctaConfig, env func(string) string) (string, int) {
	timeoutSeconds := 300
	var (
		queueCfg      *config.SandboxApprovalQueue
		approvalStore *string
	)
	if cfg != nil && cfg.Security != nil {
		queueCfg = cfg.Security.ApprovalQueue
		if cfg.Security.Sandbox != nil {
			approvalStore = cfg.Security.Sandbox.ApprovalStore
		}
	}
	if queueCfg != nil && queueCfg.TimeoutSeconds != nil && *queueCfg.TimeoutSeconds > 0 {
		timeoutSeconds = *queueCfg.TimeoutSeconds
	}
	if approvalStore != nil && strings.TrimSpace(*approvalStore) != "" {
		p := strings.TrimSpace(*approvalStore)
		if !strings.HasSuffix(strings.ToLower(p), ".json") {
			return filepath.Join(p, "approvals.json"), timeoutSeconds
		}
		return p, timeoutSeconds
	}
	stateDir := paths.ResolveStateDir(env)
	return filepath.Join(stateDir, "agents", "approvals", "approvals.json"), timeoutSeconds
}

func loadApprovalSnapshot(path string) (*approvalSnapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &approvalSnapshot{Records: nil, Whitelist: map[string]time.Time{}}, nil
		}
		return nil, err
	}
	var snap approvalSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, err
	}
	if snap.Whitelist == nil {
		snap.Whitelist = map[string]time.Time{}
	}
	return &snap, nil
}

func toListEntry(rec approvalRecord, now time.Time, timeoutSeconds int) map[string]interface{} {
	createdAt := rec.RequestedAt
	timeoutAt := createdAt.Add(time.Duration(timeoutSeconds) * time.Second)
	status := rec.State
	if status == string(security.ApprovalPending) && timeoutSeconds > 0 && now.After(timeoutAt) {
		status = "expired"
	}

	out := map[string]interface{}{
		"id":        rec.ID,
		"sessionId": rec.SessionID,
		"command":   rec.Command,
		"createdAt": createdAt.UnixMilli(),
		"timeoutAt": timeoutAt.UnixMilli(),
		"status":    status,
	}
	if rec.ExpiresAt != nil && rec.State == string(security.ApprovalApproved) {
		ttl := int(rec.ExpiresAt.Sub(now).Seconds())
		if ttl < 0 {
			ttl = 0
		}
		out["ttlSeconds"] = ttl
	}
	return out
}

// ApprovalsListHandler handles "approvals.list".
func ApprovalsListHandler(opts HandlerOpts) error {
	cfg := loadConfigFromContext(opts.Context)
	env := func(k string) string {
		// Prefer OS env for path resolution
		return os.Getenv(k)
	}
	storeFile, timeoutSeconds := resolveApprovalStoreFile(cfg, env)

	snap, err := loadApprovalSnapshot(storeFile)
	if err != nil {
		opts.Respond(false, nil, &protocol.ErrorShape{
			Code:    protocol.ErrCodeInternal,
			Message: err.Error(),
		}, nil)
		return nil
	}

	now := time.Now()
	list := make([]map[string]interface{}, 0, len(snap.Records))
	for _, rec := range snap.Records {
		list = append(list, toListEntry(rec, now, timeoutSeconds))
	}

	opts.Respond(true, map[string]interface{}{
		"storePath": storeFile,
		"entries":   list,
	}, nil, nil)
	return nil
}

// ApprovalsApproveHandler handles "approvals.approve".
// This approves the request WITHOUT adding to whitelist (ttl=0).
func ApprovalsApproveHandler(opts HandlerOpts) error {
	requestID, _ := opts.Params["requestId"].(string)
	approverID, _ := opts.Params["approverId"].(string)
	if requestID == "" {
		opts.Respond(false, nil, &protocol.ErrorShape{
			Code:    protocol.ErrCodeInvalidRequest,
			Message: "requestId required",
		}, nil)
		return nil
	}

	cfg := loadConfigFromContext(opts.Context)
	env := func(k string) string { return os.Getenv(k) }
	storeFile, timeoutSeconds := resolveApprovalStoreFile(cfg, env)
	snap, err := loadApprovalSnapshot(storeFile)
	if err != nil {
		opts.Respond(false, nil, &protocol.ErrorShape{Code: protocol.ErrCodeInternal, Message: err.Error()}, nil)
		return nil
	}
	now := time.Now()
	for _, rec := range snap.Records {
		if rec.ID == requestID && rec.State == string(security.ApprovalPending) && timeoutSeconds > 0 {
			if now.After(rec.RequestedAt.Add(time.Duration(timeoutSeconds) * time.Second)) {
				opts.Respond(false, nil, &protocol.ErrorShape{
					Code:    protocol.ErrCodeInvalidRequest,
					Message: "approval request expired",
				}, nil)
				return nil
			}
			break
		}
	}

	q, err := octasecurity.GetApprovalQueue(storeFile)
	if err != nil {
		opts.Respond(false, nil, &protocol.ErrorShape{Code: protocol.ErrCodeInternal, Message: err.Error()}, nil)
		return nil
	}
	// Approve without adding to whitelist (ttl=0)
	// 判断是否为空
	var ttl time.Duration
	if opts.Context.Config.Security != nil && opts.Context.Config.Security.ApprovalQueue != nil && opts.Context.Config.Security.ApprovalQueue.TimeoutSeconds != nil {
		ttl = time.Duration(int64(*opts.Context.Config.Security.ApprovalQueue.TimeoutSeconds)) * time.Second
	} else {
		ttl = time.Minute * 5
	}
	if _, err := q.Approve(requestID, strings.TrimSpace(approverID), ttl); err != nil {
		opts.Respond(false, nil, &protocol.ErrorShape{
			Code:    protocol.ErrCodeNotFound,
			Message: err.Error(),
		}, nil)
		return nil
	}

	opts.Respond(true, map[string]interface{}{"requestId": requestID, "status": "approved"}, nil, nil)
	return nil
}

// ApprovalsDenyHandler handles "approvals.deny".
func ApprovalsDenyHandler(opts HandlerOpts) error {
	requestID, _ := opts.Params["requestId"].(string)
	approverID, _ := opts.Params["approverId"].(string)
	reason, _ := opts.Params["reason"].(string)
	if requestID == "" {
		opts.Respond(false, nil, &protocol.ErrorShape{
			Code:    protocol.ErrCodeInvalidRequest,
			Message: "requestId required",
		}, nil)
		return nil
	}

	cfg := loadConfigFromContext(opts.Context)
	env := func(k string) string { return os.Getenv(k) }
	storeFile, timeoutSeconds := resolveApprovalStoreFile(cfg, env)
	snap, err := loadApprovalSnapshot(storeFile)
	if err != nil {
		opts.Respond(false, nil, &protocol.ErrorShape{Code: protocol.ErrCodeInternal, Message: err.Error()}, nil)
		return nil
	}
	now := time.Now()
	for _, rec := range snap.Records {
		if rec.ID == requestID && rec.State == string(security.ApprovalPending) && timeoutSeconds > 0 {
			if now.After(rec.RequestedAt.Add(time.Duration(timeoutSeconds) * time.Second)) {
				opts.Respond(false, nil, &protocol.ErrorShape{
					Code:    protocol.ErrCodeInvalidRequest,
					Message: "approval request expired",
				}, nil)
				return nil
			}
			break
		}
	}

	q, err := octasecurity.GetApprovalQueue(storeFile)
	if err != nil {
		opts.Respond(false, nil, &protocol.ErrorShape{Code: protocol.ErrCodeInternal, Message: err.Error()}, nil)
		return nil
	}
	if _, err := q.Deny(requestID, strings.TrimSpace(approverID), strings.TrimSpace(reason)); err != nil {
		opts.Respond(false, nil, &protocol.ErrorShape{
			Code:    protocol.ErrCodeNotFound,
			Message: err.Error(),
		}, nil)
		return nil
	}

	opts.Respond(true, map[string]interface{}{"requestId": requestID, "status": "denied"}, nil, nil)
	return nil
}

// ApprovalsWhitelistSessionHandler handles "approvals.whitelistSession".
// It adds the session of the given request to the whitelist (indefinitely)
// and marks this specific approval as approved.
func ApprovalsWhitelistSessionHandler(opts HandlerOpts) error {
	requestID, _ := opts.Params["requestId"].(string)
	approverID, _ := opts.Params["approverId"].(string)
	if requestID == "" {
		opts.Respond(false, nil, &protocol.ErrorShape{
			Code:    protocol.ErrCodeInvalidRequest,
			Message: "requestId required",
		}, nil)
		return nil
	}

	cfg := loadConfigFromContext(opts.Context)
	env := func(k string) string { return os.Getenv(k) }
	storeFile, timeoutSeconds := resolveApprovalStoreFile(cfg, env)
	snap, err := loadApprovalSnapshot(storeFile)
	if err != nil {
		opts.Respond(false, nil, &protocol.ErrorShape{Code: protocol.ErrCodeInternal, Message: err.Error()}, nil)
		return nil
	}

	now := time.Now()
	var rec *approvalRecord
	for i := range snap.Records {
		r := &snap.Records[i]
		if r.ID == requestID {
			// Only allow whitelisting pending, non-expired approvals
			if r.State != string(security.ApprovalPending) {
				opts.Respond(false, nil, &protocol.ErrorShape{
					Code:    protocol.ErrCodeInvalidRequest,
					Message: "approval not pending",
				}, nil)
				return nil
			}
			if timeoutSeconds > 0 && now.After(r.RequestedAt.Add(time.Duration(timeoutSeconds)*time.Second)) {
				opts.Respond(false, nil, &protocol.ErrorShape{
					Code:    protocol.ErrCodeInvalidRequest,
					Message: "approval request expired",
				}, nil)
				return nil
			}
			rec = r
			break
		}
	}
	if rec == nil {
		opts.Respond(false, nil, &protocol.ErrorShape{
			Code:    protocol.ErrCodeNotFound,
			Message: "approval request not found",
		}, nil)
		return nil
	}

	q, err := octasecurity.GetApprovalQueue(storeFile)
	if err != nil {
		opts.Respond(false, nil, &protocol.ErrorShape{Code: protocol.ErrCodeInternal, Message: err.Error()}, nil)
		return nil
	}

	// Whitelist the session indefinitely (ttl=0 means no expiry).
	var ttl time.Duration
	if opts.Context.Config.Security != nil && opts.Context.Config.Security.ApprovalQueue != nil && opts.Context.Config.Security.ApprovalQueue.TimeoutSeconds != nil {
		ttl = time.Duration(int64(*opts.Context.Config.Security.ApprovalQueue.TimeoutSeconds)) * time.Second
	} else {
		ttl = time.Minute * 5
	}

	if err := q.AddSessionToWhitelist(rec.SessionID, ttl); err != nil {
		opts.Respond(false, nil, &protocol.ErrorShape{
			Code:    protocol.ErrCodeInternal,
			Message: err.Error(),
		}, nil)
		return nil
	}

	// Also mark this pending approval as approved with configured TTL.
	if _, err := q.Approve(requestID, strings.TrimSpace(approverID), ttl); err != nil {
		opts.Respond(false, nil, &protocol.ErrorShape{
			Code:    protocol.ErrCodeInternal,
			Message: err.Error(),
		}, nil)
		return nil
	}

	opts.Respond(true, map[string]interface{}{
		"requestId": requestID,
		"sessionId": rec.SessionID,
		"status":    "whitelisted",
	}, nil, nil)
	return nil
}
