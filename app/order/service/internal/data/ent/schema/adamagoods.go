package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/schema/field"
)

// AdamaGoods holds the schema definition for the AdamaGoods entity.
type AdamaGoods struct {
	ent.Schema
}

// Fields of the AdamaGoods.
func (AdamaGoods) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id"),
		field.Int64("goods_id"),
		field.Float("adama_price").SchemaType(map[string]string{
			dialect.MySQL: "decimal(10,2)",
		}),
		field.Int64("stock_count"),
		field.Time("start_date"),
		field.Time("end_date"),
	}
}

// Edges of the AdamaGoods.
func (AdamaGoods) Edges() []ent.Edge {
	return nil
}
