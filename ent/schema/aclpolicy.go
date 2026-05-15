package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
)

// ACLPolicy holds the schema definition for the ACLPolicy entity.
type ACLPolicy struct {
	ent.Schema
}

// Fields of the ACLPolicy.
func (ACLPolicy) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").
			Default("default"),
		field.Text("policy").
			NotEmpty(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

// Edges of the ACLPolicy.
func (ACLPolicy) Edges() []ent.Edge {
	return nil
}