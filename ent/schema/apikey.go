package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
)

// APIKey holds the schema definition for the APIKey entity.
type APIKey struct {
	ent.Schema
}

// Fields of the APIKey.
func (APIKey) Fields() []ent.Field {
	return []ent.Field{
		field.String("prefix").
			Unique().
			NotEmpty(),
		field.String("key").
			NotEmpty(),
		field.Time("created_at").
			Default(time.Now),
		field.Time("expiration").
			Optional().
			Nillable(),
	}
}

// Edges of the APIKey.
func (APIKey) Edges() []ent.Edge {
	return nil
}