package schema

import (
	"fmt"

	schemapb "github.com/TBD54566975/ftl/backend/protos/xyz/block/ftl/v1/schema"
)

func posFromProto(pos *schemapb.Position) Position {
	if pos == nil {
		return Position{}
	}
	return Position{
		Line:     int(pos.Line),
		Column:   int(pos.Column),
		Filename: pos.Filename,
	}
}

func declListToSchema(s []*schemapb.Decl) []Decl {
	var out []Decl
	for _, n := range s {
		switch n := n.Value.(type) {
		case *schemapb.Decl_Verb:
			out = append(out, VerbFromProto(n.Verb))
		case *schemapb.Decl_Data:
			out = append(out, DataFromProto(n.Data))
		case *schemapb.Decl_Database:
			out = append(out, DatabaseFromProto(n.Database))
		case *schemapb.Decl_Enum:
			out = append(out, EnumFromProto(n.Enum))
		}
	}
	return out
}

func typeToSchema(s *schemapb.Type) Type {
	switch s := s.Value.(type) {
	// case *schemapb.Type_VerbRef:
	// 	return verbRefToSchema(s.VerbRef)
	case *schemapb.Type_DataRef:
		return DataRefFromProto(s.DataRef)
	case *schemapb.Type_Int:
		return &Int{Pos: posFromProto(s.Int.Pos)}
	case *schemapb.Type_Float:
		return &Float{Pos: posFromProto(s.Float.Pos)}
	case *schemapb.Type_String_:
		return &String{Pos: posFromProto(s.String_.Pos)}
	case *schemapb.Type_Bytes:
		return &Bytes{Pos: posFromProto(s.Bytes.Pos)}
	case *schemapb.Type_Time:
		return &Time{Pos: posFromProto(s.Time.Pos)}
	case *schemapb.Type_Bool:
		return &Bool{Pos: posFromProto(s.Bool.Pos)}
	case *schemapb.Type_Array:
		return arrayToSchema(s.Array)
	case *schemapb.Type_Map:
		return mapToSchema(s.Map)
	case *schemapb.Type_Optional:
		return &Optional{Pos: posFromProto(s.Optional.Pos), Type: typeToSchema(s.Optional.Type)}
	case *schemapb.Type_Unit:
		return &Unit{Pos: posFromProto(s.Unit.Pos)}
	case *schemapb.Type_Any:
		return &Any{Pos: posFromProto(s.Any.Pos)}
	}
	panic(fmt.Sprintf("unhandled type: %T", s.Value))
}

func valueToSchema(v *schemapb.Value) Value {
	switch s := v.Value.(type) {
	case *schemapb.Value_IntValue:
		return &IntValue{
			Pos:   posFromProto(s.IntValue.Pos),
			Value: int(s.IntValue.Value),
		}
	case *schemapb.Value_StringValue:
		return &StringValue{
			Pos:   posFromProto(s.StringValue.Pos),
			Value: s.StringValue.GetValue(),
		}
	}
	panic(fmt.Sprintf("unhandled schema value: %T", v.Value))
}

func metadataListToSchema(s []*schemapb.Metadata) []Metadata {
	var out []Metadata
	for _, n := range s {
		out = append(out, metadataToSchema(n))
	}
	return out
}

func metadataToSchema(s *schemapb.Metadata) Metadata {
	switch s := s.Value.(type) {
	case *schemapb.Metadata_Calls:
		return &MetadataCalls{
			Pos:   posFromProto(s.Calls.Pos),
			Calls: verbRefListToSchema(s.Calls.Calls),
		}

	case *schemapb.Metadata_Databases:
		return &MetadataDatabases{
			Pos:   posFromProto(s.Databases.Pos),
			Calls: databaseListToSchema(s.Databases.Calls),
		}

	case *schemapb.Metadata_Ingress:
		return &MetadataIngress{
			Pos:    posFromProto(s.Ingress.Pos),
			Type:   s.Ingress.Type,
			Method: s.Ingress.Method,
			Path:   ingressPathComponentListToSchema(s.Ingress.Path),
		}

	default:
		panic(fmt.Sprintf("unhandled metadata type: %T", s))
	}
}
