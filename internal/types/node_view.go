package types

import (
	"net/netip"
	"slices"
	"time"

	"github.com/juanfont/headscale-v2/internal/policy/matcher"
	"go4.org/netipx"
	"tailscale.com/tailcfg"
	"tailscale.com/types/key"
	"tailscale.com/types/views"
)

type NodeView struct {
	ж *Node
}

func (n Node) View() NodeView {
	return NodeView{ж: &n}
}

func (nv NodeView) Valid() bool {
	return nv.ж != nil
}

// AsStruct returns a clone of the underlying Node value.
func (nv NodeView) AsStruct() *Node {
	if nv.ж == nil {
		return nil
	}
	clone := *nv.ж
	return &clone
}

func (nv NodeView) ID() NodeID {
	return nv.ж.ID
}

func (nv NodeView) MachineKey() key.MachinePublic {
	return nv.ж.MachineKey
}

func (nv NodeView) NodeKey() key.NodePublic {
	return nv.ж.NodeKey
}

func (nv NodeView) DiscoKey() key.DiscoPublic {
	return nv.ж.DiscoKey
}

func (nv NodeView) Endpoints() views.Slice[netip.AddrPort] {
	return views.SliceOf(nv.ж.Endpoints)
}

func (nv NodeView) IPv4() netip.Addr {
	if nv.ж.IPv4 == nil {
		return netip.Addr{}
	}
	return *nv.ж.IPv4
}

func (nv NodeView) IPv6() netip.Addr {
	if nv.ж.IPv6 == nil {
		return netip.Addr{}
	}
	return *nv.ж.IPv6
}

func (nv NodeView) Hostname() string {
	return nv.ж.Hostname
}

func (nv NodeView) GivenName() string {
	return nv.ж.GivenName
}

func (nv NodeView) User() UserView {
	if nv.ж.User == nil {
		return UserView{}
	}
	return nv.ж.User.View()
}

func (nv NodeView) Tags() views.Slice[string] {
	return views.SliceOf(nv.ж.Tags)
}

func (nv NodeView) Expiry() Option[time.Time] {
	if nv.ж.Expiry == nil {
		return None[time.Time]()
	}
	return Some(*nv.ж.Expiry)
}

func (nv NodeView) LastSeen() Option[time.Time] {
	if nv.ж.LastSeen == nil {
		return None[time.Time]()
	}
	return Some(*nv.ж.LastSeen)
}

func (nv NodeView) CreatedAt() time.Time {
	return nv.ж.CreatedAt
}

func (nv NodeView) IsOnline() Option[bool] {
	if nv.ж.IsOnline == nil {
		return None[bool]()
	}
	return Some(*nv.ж.IsOnline)
}

func (nv NodeView) IsExpired() bool {
	return nv.ж.IsExpired()
}

func (nv NodeView) IsEphemeral() bool {
	return nv.ж.IsEphemeral()
}

func (nv NodeView) IsTagged() bool {
	return nv.ж.IsTagged()
}

func (nv NodeView) IPs() []netip.Addr {
	return nv.ж.IPs()
}

func (nv NodeView) Prefixes() []netip.Prefix {
	return nv.ж.Prefixes()
}

func (nv NodeView) AnnouncedRoutes() []netip.Prefix {
	return nv.ж.AnnouncedRoutes()
}

func (nv NodeView) SubnetRoutes() []netip.Prefix {
	return nv.ж.SubnetRoutes()
}

func (nv NodeView) ExitRoutes() []netip.Prefix {
	return nv.ж.ExitRoutes()
}

func (nv NodeView) AllApprovedRoutes() []netip.Prefix {
	return nv.ж.AllApprovedRoutes()
}

func (nv NodeView) IsExitNode() bool {
	return nv.ж.IsExitNode()
}

// IsSubnetRouter returns true if the node is advertising subnet routes.
func (nv NodeView) IsSubnetRouter() bool {
	if !nv.Valid() {
		return false
	}
	return len(nv.SubnetRoutes()) > 0 || len(nv.AllApprovedRoutes()) > 0
}

func (nv NodeView) TypedUserID() UserID {
	return nv.ж.TypedUserID()
}

func (nv NodeView) AppendToIPSet(builder *netipx.IPSetBuilder) {
	if !nv.Valid() {
		return
	}
	nv.ж.AppendToIPSet(builder)
}

func (nv NodeView) InIPSet(ipSet *netipx.IPSet) bool {
	if !nv.Valid() || ipSet == nil {
		return false
	}
	for _, ip := range nv.IPs() {
		if ipSet.Contains(ip) {
			return true
		}
	}
	return false
}

func (nv NodeView) HasTag(tag string) bool {
	if !nv.Valid() {
		return false
	}
	return nv.ж.HasTag(tag)
}

func (nv NodeView) CanAccess(matchers []matcher.Match, node2 NodeView) bool {
	if !nv.Valid() || !node2.Valid() {
		return false
	}
	return nv.ж.CanAccess(matchers, node2.ж)
}

func (nv NodeView) CanAccessRoute(matchers []matcher.Match, route netip.Prefix) bool {
	if !nv.Valid() {
		return false
	}
	return nv.ж.CanAccessRoute(matchers, route)
}

func (nv NodeView) String() string {
	if !nv.Valid() {
		return ""
	}
	return nv.ж.String()
}

// HasNetworkChanges checks if the node has network-related changes.
func (nv NodeView) HasNetworkChanges(other NodeView) bool {
	if !nv.Valid() || !other.Valid() {
		return false
	}
	if !slices.Equal(nv.IPs(), other.IPs()) {
		return true
	}
	if !equalPrefixesUnordered(nv.AnnouncedRoutes(), other.AnnouncedRoutes()) {
		return true
	}
	if !equalPrefixesUnordered(nv.SubnetRoutes(), other.SubnetRoutes()) {
		return true
	}
	if !equalPrefixesUnordered(nv.ExitRoutes(), other.ExitRoutes()) {
		return true
	}
	return false
}

// HasPolicyChange reports whether the node has changes that affect policy evaluation.
func (nv NodeView) HasPolicyChange(other NodeView) bool {
	if !nv.Valid() || !other.Valid() {
		return false
	}
	if nv.UserID() != other.UserID() {
		return true
	}
	if !slices.Equal(nv.Tags().AsSlice(), other.Tags().AsSlice()) {
		return true
	}
	if !slices.Equal(nv.IPs(), other.IPs()) {
		return true
	}
	if !equalPrefixesUnordered(nv.SubnetRoutes(), other.SubnetRoutes()) {
		return true
	}
	return false
}

// equalPrefixesUnordered reports whether a and b contain the same prefixes, order-independent.
func equalPrefixesUnordered(a, b []netip.Prefix) bool {
	if len(a) != len(b) {
		return false
	}
	ac := slices.Clone(a)
	bc := slices.Clone(b)
	slices.SortFunc(ac, netip.Prefix.Compare)
	slices.SortFunc(bc, netip.Prefix.Compare)
	return slices.Equal(ac, bc)
}

// UserID returns the user ID pointer for the node.
func (nv NodeView) UserID() *uint {
	if !nv.Valid() {
		return nil
	}
	return nv.ж.UserID
}

type Option[T any] struct {
	val   T
	valid bool
}

func Some[T any](v T) Option[T] {
	return Option[T]{val: v, valid: true}
}

func None[T any]() Option[T] {
	return Option[T]{valid: false}
}

func (o Option[T]) Valid() bool {
	return o.valid
}

func (o Option[T]) Get() T {
	return o.val
}

func (o Option[T]) Clone() Option[T] {
	return o
}

type UserView struct {
	ж *User
}

func (u User) View() UserView {
	return UserView{ж: &u}
}

func (uv UserView) Valid() bool {
	return uv.ж != nil
}

func (uv UserView) ID() UserID {
	return uv.ж.ID
}

func (uv UserView) Name() string {
	return uv.ж.Name
}

func (uv UserView) DisplayName() string {
	return uv.ж.DisplayName
}

func (uv UserView) Email() string {
	return uv.ж.Email
}

func (uv UserView) Model() *User {
	if uv.ж == nil {
		return nil
	}
	return uv.ж
}

func (uv UserView) TailscaleUserProfile() tailcfg.UserProfile {
	return tailcfg.UserProfile{
		ID:          tailcfg.UserID(uv.ж.ID),
		LoginName:   uv.ж.Username(),
		DisplayName: uv.ж.Display(),
	}
}

// Owner returns the owner of the node. For tagged nodes, returns TaggedDevices.
func (nv NodeView) Owner() UserView {
	if !nv.Valid() {
		return UserView{}
	}
	if nv.IsTagged() {
		// Return a synthetic TaggedDevices user
		return UserView{ж: &User{ID: TaggedDevicesUser.ID, Name: TaggedDevicesUser.Name}}
	}
	if user := nv.User(); user.Valid() {
		return user
	}
	return UserView{}
}

func (nodes Nodes) ViewSlice() views.Slice[NodeView] {
	vs := make([]NodeView, len(nodes))
	for i, n := range nodes {
		vs[i] = n.View()
	}
	return views.SliceOf(vs)
}

func (nv NodeView) TailNode(
	capVer tailcfg.CapabilityVersion,
	primaryRouteFunc RouteFunc,
	cfg *Config,
) (*tailcfg.Node, error) {
	if !nv.Valid() {
		return nil, ErrInvalidNodeView
	}

	hostname := nv.ж.Hostname

	var keyExpiry time.Time
	if nv.Expiry().Valid() {
		keyExpiry = nv.Expiry().Get()
	}

	allRoutes := primaryRouteFunc(nv.ID())
	allowedIPs := slices.Concat(nv.Prefixes(), allRoutes)

	var primaryRoutes []netip.Prefix
	for _, r := range allRoutes {
		if r.Bits() < r.Addr().BitLen() {
			primaryRoutes = append(primaryRoutes, r)
		}
	}

	var online *bool
	if nv.ж.IsOnline != nil {
		online = nv.ж.IsOnline
	}

	tNode := &tailcfg.Node{
		ID:       tailcfg.NodeID(nv.ID()),
		StableID: nv.ID().StableID(),
		Name:     hostname,
		Cap:      capVer,

		Key:       nv.NodeKey(),
		KeyExpiry: keyExpiry.UTC(),

		Machine:      nv.MachineKey(),
		DiscoKey:     nv.DiscoKey(),
		Addresses:    nv.Prefixes(),
		PrimaryRoutes: primaryRoutes,
		AllowedIPs:   allowedIPs,
		Endpoints:    nv.Endpoints().AsSlice(),
		Created:      nv.CreatedAt().UTC(),

		Online: online,

		Tags: nv.Tags().AsSlice(),

		MachineAuthorized: !nv.IsExpired(),
		Expired:           nv.IsExpired(),
	}

	if nv.LastSeen().Valid() && online != nil && !*online {
		lastSeen := nv.LastSeen().Get()
		tNode.LastSeen = &lastSeen
	}

	return tNode, nil
}
