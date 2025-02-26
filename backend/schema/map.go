package schema

import (
	"fmt"

	"google.golang.org/protobuf/proto"

	schemapb "github.com/TBD54566975/ftl/backend/protos/xyz/block/ftl/v1/schema"
)

type Map struct {
	Pos Position `parser:"" protobuf:"1,optional"`

	Key   Type `parser:"'{' @@" protobuf:"2"`
	Value Type `parser:"':' @@ '}'" protobuf:"3"`
}

var _ Type = (*Map)(nil)

func (m *Map) Position() Position     { return m.Pos }
func (m *Map) schemaChildren() []Node { return []Node{m.Key, m.Value} }
func (*Map) schemaType()              {}
func (m *Map) String() string         { return fmt.Sprintf("{%s: %s}", m.Key.String(), m.Value.String()) }

func (m *Map) ToProto() proto.Message {
	return &schemapb.Map{
		Key:   typeToProto(m.Key),
		Value: typeToProto(m.Value),
	}
}

func mapToSchema(s *schemapb.Map) *Map {
	return &Map{
		Pos:   posFromProto(s.Pos),
		Key:   typeToSchema(s.Key),
		Value: typeToSchema(s.Value),
	}
}
