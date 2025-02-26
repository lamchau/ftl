package compile

import (
	"go/ast"
	"go/types"
	"strings"
	"testing"

	"github.com/alecthomas/assert/v2"
	"github.com/alecthomas/participle/v2/lexer"

	"github.com/TBD54566975/ftl/backend/schema"
	"github.com/TBD54566975/ftl/internal/slices"
)

func TestExtractModuleSchema(t *testing.T) {
	_, actual, err := ExtractModuleSchema("testdata/one")
	assert.NoError(t, err)
	actual = schema.Normalise(actual)
	expected := `module one {
  data Nested {
  }

  data Req {
    int Int
    int64 Int
    float Float
    string String
    slice [String]
    map {String: String}
    nested one.Nested
    optional one.Nested?
    time Time
    user two.User alias json "u"
    bytes Bytes
  }

  data Resp {
  }

  verb verb(one.Req) one.Resp
}
`
	assert.Equal(t, expected, actual.String())
}

func TestExtractModuleSchemaTwo(t *testing.T) {
	_, actual, err := ExtractModuleSchema("testdata/two")
	assert.NoError(t, err)
	actual = schema.Normalise(actual)
	expected := `module two {
  data Payload<T> {
    body T
  }

  verb two(two.Payload<String>) two.Payload<String>

  verb callsTwo(two.Payload<String>) two.Payload<String>
      calls two.two

}
`
	assert.Equal(t, normaliseString(expected), normaliseString(actual.String()))
}

func TestParseDirectives(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected directive
	}{
		{name: "Module", input: "ftl:module foo", expected: &directiveModule{Name: "foo"}},
		{name: "Verb", input: "ftl:verb", expected: &directiveVerb{Verb: true}},
		{name: "Ingress", input: `ftl:ingress GET /foo`, expected: &directiveIngress{
			MetadataIngress: schema.MetadataIngress{
				Method: "GET",
				Path: []schema.IngressPathComponent{
					&schema.IngressPathLiteral{
						Text: "foo",
					},
				},
			},
		}},
		{name: "Ingress", input: `ftl:ingress GET /test_path/{something}/987-Your_File.txt%7E%21Misc%2A%28path%29info%40abc%3Fxyz`, expected: &directiveIngress{
			MetadataIngress: schema.MetadataIngress{
				Method: "GET",
				Path: []schema.IngressPathComponent{
					&schema.IngressPathLiteral{
						Text: "test_path",
					},
					&schema.IngressPathParameter{
						Name: "something",
					},
					&schema.IngressPathLiteral{
						Text: "987-Your_File.txt%7E%21Misc%2A%28path%29info%40abc%3Fxyz",
					},
				},
			},
		}},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			got, err := directiveParser.ParseString("", tt.input)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, got.Directive, assert.Exclude[lexer.Position](), assert.Exclude[schema.Position]())
		})
	}
}

func TestParseTypesTime(t *testing.T) {
	timeRef := mustLoadRef("time", "Time").Type()
	parsed, err := visitType(nil, &ast.Ident{}, timeRef)
	assert.NoError(t, err)
	_, ok := parsed.(*schema.Time)
	assert.True(t, ok)
}

func TestParseBasicTypes(t *testing.T) {
	tests := []struct {
		name     string
		input    types.Type
		expected schema.Type
	}{
		{name: "String", input: types.Typ[types.String], expected: &schema.String{}},
		{name: "Int", input: types.Typ[types.Int], expected: &schema.Int{}},
		{name: "Bool", input: types.Typ[types.Bool], expected: &schema.Bool{}},
		{name: "Float64", input: types.Typ[types.Float64], expected: &schema.Float{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := visitType(nil, &ast.Ident{}, tt.input)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, parsed)
		})
	}
}

func normaliseString(s string) string {
	return strings.TrimSpace(strings.Join(slices.Map(strings.Split(s, "\n"), strings.TrimSpace), "\n"))
}
