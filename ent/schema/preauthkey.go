package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// PreAuthKey holds the schema definition for the PreAuthKey entity.
type PreAuthKey struct {
	ent.Schema
}

// Fields of the PreAuthKey.
func (PreAuthKey) Fields() []ent.Field {
	return []ent.Field{
		field.String("key").
			Unique().
			NotEmpty(),
		field.Bool("reusable").
			Default(false),
		field.Bool("ephemeral").
			Default(false),
		field.Int("used_count").
			Default(0),
		field.Time("expiration").
			Optional().
			Nillable(),
		field.Time("created_at").
			Default(time.Now),
		field.Strings("acl_tags").
			Optional().
			Comment("ACL tags to apply to nodes registered with this key"),
	}
}

// Edges of the PreAuthKey.
func (PreAuthKey) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("pre_auth_keys").
			Unique(),
		edge.To("nodes", Node.Type),
	}
}