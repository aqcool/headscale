package types

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"tailscale.com/tailcfg"
)

const (
	SelfUpdateIdentifier = "self-update"
	DatabasePostgres     = "postgres"
	DatabaseSqlite       = "sqlite3"

	HTTPTimeout            = 30 * time.Second
	HTTPShutdownTimeout    = 3 * time.Second
	KeepAliveInterval      = 60 * time.Second
	MaxHostnameLength      = 255

	authIDPrefix       = "hskey-authreq-"
	authIDRandomLength = 24
	AuthIDLength       = 38
)

var (
	ErrCannotParsePrefix   = errors.New("cannot parse prefix")
	ErrInvalidAuthIDLength = errors.New("auth ID has invalid length")
	ErrInvalidAuthIDPrefix = errors.New("auth ID has invalid prefix")
)

type StateUpdateType int

func (su StateUpdateType) String() string {
	switch su {
	case StateFullUpdate:
		return "StateFullUpdate"
	case StatePeerChanged:
		return "StatePeerChanged"
	case StatePeerChangedPatch:
		return "StatePeerChangedPatch"
	case StatePeerRemoved:
		return "StatePeerRemoved"
	case StateSelfUpdate:
		return "StateSelfUpdate"
	case StateDERPUpdated:
		return "StateDERPUpdated"
	}
	return "unknown state update type"
}

const (
	StateFullUpdate StateUpdateType = iota
	StatePeerChanged
	StatePeerChangedPatch
	StatePeerRemoved
	StateSelfUpdate
	StateDERPUpdated
)

type StateUpdate struct {
	Type StateUpdateType

	ChangeNodes []NodeID
	ChangePatches []*tailcfg.PeerChange
	Removed []NodeID
	DERPMap *tailcfg.DERPMap
	Message string
}

func (su *StateUpdate) Empty() bool {
	switch su.Type {
	case StatePeerChanged:
		return len(su.ChangeNodes) == 0
	case StatePeerChangedPatch:
		return len(su.ChangePatches) == 0
	case StatePeerRemoved:
		return len(su.Removed) == 0
	case StateFullUpdate, StateSelfUpdate, StateDERPUpdated:
		return false
	}
	return false
}

func UpdateFull() StateUpdate {
	return StateUpdate{Type: StateFullUpdate}
}

func UpdateSelf(nodeID NodeID) StateUpdate {
	return StateUpdate{
		Type:        StateSelfUpdate,
		ChangeNodes: []NodeID{nodeID},
	}
}

func UpdatePeerChanged(nodeIDs ...NodeID) StateUpdate {
	return StateUpdate{
		Type:        StatePeerChanged,
		ChangeNodes: nodeIDs,
	}
}

func UpdatePeerPatch(changes ...*tailcfg.PeerChange) StateUpdate {
	return StateUpdate{
		Type:          StatePeerChangedPatch,
		ChangePatches: changes,
	}
}

func UpdatePeerRemoved(nodeIDs ...NodeID) StateUpdate {
	return StateUpdate{
		Type:    StatePeerRemoved,
		Removed: nodeIDs,
	}
}

func UpdateExpire(nodeID NodeID, expiry time.Time) StateUpdate {
	return StateUpdate{
		Type: StatePeerChangedPatch,
		ChangePatches: []*tailcfg.PeerChange{
			{
				NodeID:    nodeID.NodeID(),
				KeyExpiry: &expiry,
			},
		},
	}
}

type AuthID string

func NewAuthID() (AuthID, error) {
	rid, err := generateRandomStringURLSafe(authIDRandomLength)
	if err != nil {
		return "", err
	}
	return AuthID(authIDPrefix + rid), nil
}

func (r AuthID) String() string {
	return string(r)
}

func (r AuthID) Validate() error {
	if !strings.HasPrefix(string(r), authIDPrefix) {
		return fmt.Errorf("%w: expected prefix %q", ErrInvalidAuthIDPrefix, authIDPrefix)
	}
	if len(r) != AuthIDLength {
		return fmt.Errorf("%w: expected %d, got %d", ErrInvalidAuthIDLength, AuthIDLength, len(r))
	}
	return nil
}

func generateRandomStringURLSafe(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b)[:n], nil
}

func MustAuthID() AuthID {
	rid, err := NewAuthID()
	if err != nil {
		panic(err)
	}
	return rid
}

type AuthVerdict struct {
	Err error
}

type SSHCheckBinding struct {
	SrcNodeID NodeID
	DstNodeID NodeID
}

type PendingRegistrationConfirmation struct {
	UserID     uint
	NodeExpiry *time.Time
	CSRF       string
}

type AuthRequest struct {
	regData             *RegistrationData
	sshBinding          *SSHCheckBinding
	pendingConfirmation *PendingRegistrationConfirmation
	finished            chan AuthVerdict
	closed              *atomic.Bool
}

func NewAuthRequest() *AuthRequest {
	return &AuthRequest{
		finished: make(chan AuthVerdict, 1),
		closed:   &atomic.Bool{},
	}
}

func NewRegisterAuthRequest(data *RegistrationData) *AuthRequest {
	return &AuthRequest{
		regData:  data,
		finished: make(chan AuthVerdict, 1),
		closed:   &atomic.Bool{},
	}
}

func NewSSHCheckAuthRequest(src, dst NodeID) *AuthRequest {
	return &AuthRequest{
		sshBinding: &SSHCheckBinding{SrcNodeID: src, DstNodeID: dst},
		finished:   make(chan AuthVerdict, 1),
		closed:     &atomic.Bool{},
	}
}

func (rn *AuthRequest) RegistrationData() *RegistrationData {
	if rn.regData == nil {
		panic("RegistrationData can only be used in registration requests")
	}
	return rn.regData
}

func (rn *AuthRequest) SSHCheckBinding() *SSHCheckBinding {
	if rn.sshBinding == nil {
		panic("SSHCheckBinding can only be used in SSH check requests")
	}
	return rn.sshBinding
}

func (rn *AuthRequest) PendingConfirmation() *PendingRegistrationConfirmation {
	return rn.pendingConfirmation
}

func (rn *AuthRequest) SetPendingConfirmation(p *PendingRegistrationConfirmation) {
	rn.pendingConfirmation = p
}

func (rn *AuthRequest) IsRegistration() bool {
	return rn.regData != nil
}

func (rn *AuthRequest) IsSSHCheck() bool {
	return rn.sshBinding != nil
}

func (rn *AuthRequest) FinishAuth(verdict AuthVerdict) {
	if rn.closed.Swap(true) {
		return
	}
	rn.finished <- verdict
}

func (rn *AuthRequest) Wait() AuthVerdict {
	return <-rn.finished
}

func (rn *AuthRequest) WaitForAuth() <-chan AuthVerdict {
	return rn.finished
}

func (v AuthVerdict) Accept() bool {
	return v.Err == nil
}