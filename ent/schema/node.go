package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Node holds the schema definition for the Node entity.
type Node struct {
	ent.Schema
}

// Fields of the Node.
func (Node) Fields() []ent.Field {
	return []ent.Field{
		field.String("machine_key").
			Unique().
			NotEmpty().
			Comment("Node's MachineKey (v1)"),
		field.String("node_key").
			NotEmpty().
			Comment("Node's current NodeKey"),
		field.String("disco_key").
			Optional().
			Comment("Node's current DiscoKey"),
		field.Strings("endpoints").
			Optional().
			Comment("Node's endpoints for direct connections"),
		field.String("hostinfo").
			Optional().
			Comment("Node's Hostinfo JSON"),
		field.Strings("ip_addresses").
			Optional().
			Comment("Node's allocated IP addresses"),
		field.String("name").
			NotEmpty().
			Comment("Node's hostname from Hostinfo"),
		field.String("given_name").
			Optional().
			Unique().
			Comment("Node's assigned name"),
		field.Time("last_seen").
			Optional().
			Nillable().
			Comment("Last time node was seen online"),
		field.Time("expiry").
			Optional().
			Nillable().
			Comment("Node's key expiry time"),
		field.Time("created_at").
			Default(time.Now).
			Comment("Node registration time"),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now).
			Comment("Last update time"),
		field.String("register_method").
			Default("unspecified").
			Comment("How the node was registered"),
		field.Strings("approved_routes").
			Optional().
			Comment("Routes approved by policy"),
		field.Strings("available_routes").
			Optional().
			Comment("Routes advertised by node"),
		field.Strings("tags").
			Optional().
			Comment("Tags assigned to node"),
		field.Bool("is_online").
			Optional().
			Default(false).
			Comment("Node online status"),
		field.Bool("ephemeral").
			Optional().
			Default(false).
			Comment("Whether node is ephemeral"),
		field.Uint64("session_epoch").
			Optional().
			Default(0).
			Comment("Map session epoch"),
		field.String("node_id").
			Optional().
			Comment("Stable node ID"),
		// Foreign key field for user
		field.Int("user_id").
			Optional().
			Nillable().
			Comment("Owner user ID"),
	}
}

// Edges of the Node.
func (Node) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("nodes").
			Unique().
			Field("user_id"),
		edge.From("pre_auth_key", PreAuthKey.Type).
			Ref("nodes").
			Unique(),
	}
}

// Indexes of the Node.
func (Node) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("machine_key").Unique(),
		index.Fields("node_key"),
		index.Fields("given_name").Unique(),
		index.Fields("name"),
	}
}
