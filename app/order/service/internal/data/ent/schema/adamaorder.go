package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
)

// AdamaOrder holds the schema definition for the AdamaOrder entity.
type AdamaOrder struct {
	ent.Schema
}

// Fields of the AdamaOrder.
func (AdamaOrder) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id"),
		field.Int64("user_id"),
		field.Int64("order_id"),
		field.Int64("goods_id"),
	}
}

// Edges of the AdamaOrder.
func (AdamaOrder) Edges() []ent.Edge {
	return nil
}
