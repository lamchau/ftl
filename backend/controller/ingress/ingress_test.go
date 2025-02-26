package ingress

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/alecthomas/assert/v2"

	"github.com/TBD54566975/ftl/backend/schema"
)

type obj = map[string]any

func TestMatchAndExtractAllSegments(t *testing.T) {
	tests := []struct {
		pattern  string
		urlPath  string
		expected map[string]string
		matched  bool
	}{
		// valid patterns
		{"", "", map[string]string{}, true},
		{"/", "/", map[string]string{}, true},
		{"/{id}", "/123", map[string]string{"id": "123"}, true},
		{"/{id}/{userId}", "/123/456", map[string]string{"id": "123", "userId": "456"}, true},
		{"/users", "/users", map[string]string{}, true},
		{"/users/{id}", "/users/123", map[string]string{"id": "123"}, true},
		{"/users/{id}", "/users/123", map[string]string{"id": "123"}, true},
		{"/users/{id}/posts/{postId}", "/users/123/posts/456", map[string]string{"id": "123", "postId": "456"}, true},

		// invalid patterns
		{"/", "/users", map[string]string{}, false},
		{"/users/{id}", "/bogus/123", map[string]string{}, false},
	}

	for _, test := range tests {
		actual := make(map[string]string)
		match := matchSegments(test.pattern, test.urlPath, func(segment, value string) {
			actual[segment] = value
		})
		assert.Equal(t, test.matched, match, "pattern = %s, urlPath = %s", test.pattern, test.urlPath)
		assert.Equal(t, test.expected, actual, "pattern = %s, urlPath = %s", test.pattern, test.urlPath)
	}
}

func TestValidation(t *testing.T) {
	tests := []struct {
		name    string
		schema  string
		request obj
		err     string
	}{
		{name: "Int",
			schema:  `module test { data Test { intValue Int } }`,
			request: obj{"intValue": 10.0}},
		{name: "Float",
			schema:  `module test { data Test { floatValue Float } }`,
			request: obj{"floatValue": 10.0}},
		{name: "String",
			schema:  `module test { data Test { stringValue String } }`,
			request: obj{"stringValue": "test"}},
		{name: "Bool",
			schema:  `module test { data Test { boolValue Bool } }`,
			request: obj{"boolValue": true}},
		{name: "IntString",
			schema:  `module test { data Test { intValue Int } }`,
			request: obj{"intValue": "10"}},
		{name: "FloatString",
			schema:  `module test { data Test { floatValue Float } }`,
			request: obj{"floatValue": "10.0"}},
		{name: "BoolString",
			schema:  `module test { data Test { boolValue Bool } }`,
			request: obj{"boolValue": "true"}},
		{name: "Array",
			schema:  `module test { data Test { arrayValue [String] } }`,
			request: obj{"arrayValue": []any{"test1", "test2"}}},
		{name: "Map",
			schema:  `module test { data Test { mapValue {String: String} } }`,
			request: obj{"mapValue": obj{"key1": "value1", "key2": "value2"}}},
		{name: "DataRef",
			schema:  `module test { data Nested { intValue Int } data Test { dataRef Nested } }`,
			request: obj{"dataRef": obj{"intValue": 10.0}}},
		{name: "Optional",
			schema:  `module test { data Test { intValue Int? } }`,
			request: obj{}},
		{name: "OptionalProvided",
			schema:  `module test { data Test { intValue Int? } }`,
			request: obj{"intValue": 10.0}},
		{name: "ArrayDataRef",
			schema:  `module test { data Nested { intValue Int } data Test { arrayValue [Nested] } }`,
			request: obj{"arrayValue": []any{obj{"intValue": 10.0}, obj{"intValue": 20.0}}}},
		{name: "MapDataRef",
			schema:  `module test { data Nested { intValue Int } data Test { mapValue {String: Nested} } }`,
			request: obj{"mapValue": obj{"key1": obj{"intValue": 10.0}, "key2": obj{"intValue": 20.0}}}},
		{name: "OtherModuleRef",
			schema:  `module other { data Other { intValue Int } } module test { data Test { otherRef other.Other } }`,
			request: obj{"otherRef": obj{"intValue": 10.0}}},
		{name: "AllowedMissingFieldTypes",
			schema: `
			module test {
				data Test {
					array [Int]
					map {String: Int}
					any Any
					bytes Bytes
					unit Unit
				}
			}`,
			request: obj{}},
		{name: "RequiredFields",
			schema:  `module test { data Test { int Int } }`,
			request: obj{},
			err:     "int is required",
		},
		// TODO: More tests for invalid data.
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sch, err := schema.ParseString("", test.schema)
			assert.NoError(t, err)

			err = validateRequestMap(&schema.DataRef{Module: "test", Name: "Test"}, nil, test.request, sch)
			if test.err != "" {
				assert.EqualError(t, err, test.err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestParseQueryParams(t *testing.T) {
	data := &schema.Data{
		Fields: []*schema.Field{
			{Name: "int", Type: &schema.Int{}},
			{Name: "float", Type: &schema.Float{}},
			{Name: "string", Type: &schema.String{}},
			{Name: "bool", Type: &schema.Bool{}},
			{Name: "array", Type: &schema.Array{Element: &schema.Int{}}},
		},
	}
	tests := []struct {
		query   string
		request obj
		err     string
	}{
		{query: "", request: obj{}},
		{query: "int=1", request: obj{"int": "1"}},
		{query: "float=2.2", request: obj{"float": "2.2"}},
		{query: "string=test", request: obj{"string": "test"}},
		{query: "bool=true", request: obj{"bool": "true"}},
		{query: "array=2", request: obj{"array": []string{"2"}}},
		{query: "array=10&array=11", request: obj{"array": []string{"10", "11"}}},
		{query: "int=10&array=11&array=12", request: obj{"int": "10", "array": []string{"11", "12"}}},
		{query: "int=1&int=2", request: nil, err: "multiple values for \"int\" are not supported"},
		{query: "[a,b]=c", request: nil, err: "complex key \"[a,b]\" is not supported, use '@json=' instead"},
		{query: "array=[1,2]", request: nil, err: "complex value \"[1,2]\" is not supported, use '@json=' instead"},
	}

	for _, test := range tests {
		parsedQuery, err := url.ParseQuery(test.query)
		assert.NoError(t, err)
		actual, err := parseQueryParams(parsedQuery, data)
		assert.EqualError(t, err, test.err)
		assert.Equal(t, test.request, actual, test.query)
	}
}

func TestParseQueryJson(t *testing.T) {
	tests := []struct {
		query   string
		request obj
		err     string
	}{
		{query: "@json=", request: nil, err: "failed to parse '@json' query parameter: unexpected end of JSON input"},
		{query: "@json=10", request: nil, err: "failed to parse '@json' query parameter: json: cannot unmarshal number into Go value of type map[string]interface {}"},
		{query: "@json=10&a=b", request: nil, err: "only '@json' parameter is allowed, but other parameters were found"},
		{query: "@json=%7B%7D", request: obj{}},
		{query: `@json=%7B%22a%22%3A%2010%7D`, request: obj{"a": 10.0}},
		{query: `@json=%7B%22a%22%3A%2010%2C%20%22b%22%3A%2011%7D`, request: obj{"a": 10.0, "b": 11.0}},
		{query: `@json=%7B%22a%22%3A%20%7B%22b%22%3A%2010%7D%7D`, request: obj{"a": obj{"b": 10.0}}},
		{query: `@json=%7B%22a%22%3A%20%7B%22b%22%3A%2010%7D%2C%20%22c%22%3A%2011%7D`, request: obj{"a": obj{"b": 10.0}, "c": 11.0}},

		// also works with non-urlencoded json
		{query: `@json={"a": {"b": 10}, "c": 11}`, request: obj{"a": obj{"b": 10.0}, "c": 11.0}},
	}

	for _, test := range tests {
		parsedQuery, err := url.ParseQuery(test.query)
		assert.NoError(t, err)
		actual, err := parseQueryParams(parsedQuery, &schema.Data{})
		assert.EqualError(t, err, test.err)
		assert.Equal(t, test.request, actual, test.query)
	}
}

func TestResponseBodyForVerb(t *testing.T) {
	jsonVerb := &schema.Verb{
		Name: "Json",
		Response: &schema.DataRef{Module: "builtin", Name: "HttpResponse", TypeParameters: []schema.Type{
			&schema.DataRef{
				Module: "test",
				Name:   "Test",
			},
			&schema.String{},
		}},
	}
	stringVerb := &schema.Verb{
		Name: "String",
		Response: &schema.DataRef{Module: "builtin", Name: "HttpResponse", TypeParameters: []schema.Type{
			&schema.String{},
			&schema.String{},
		}},
	}
	sch := &schema.Schema{
		Modules: []*schema.Module{
			schema.Builtins(),
			{
				Name: "test",
				Decls: []schema.Decl{
					&schema.Data{
						Name: "Test",
						Fields: []*schema.Field{
							{Name: "message", Type: &schema.String{}, JSONAlias: "msg"},
						},
					},
					jsonVerb,
				},
			},
		},
	}
	tests := []struct {
		name            string
		verb            *schema.Verb
		headers         map[string][]string
		body            []byte
		expectedBody    []byte
		expectedHeaders http.Header
	}{
		{
			name:            "application/json",
			verb:            jsonVerb,
			headers:         map[string][]string{"Content-Type": {"application/json"}},
			body:            []byte(`{"message": "Hello, World!"}`),
			expectedBody:    []byte(`{"msg":"Hello, World!"}`),
			expectedHeaders: http.Header{"Content-Type": []string{"application/json"}},
		},
		{
			name:            "Default to application/json",
			verb:            jsonVerb,
			headers:         map[string][]string{},
			body:            []byte(`{"message": "Default to JSON"}`),
			expectedBody:    []byte(`{"msg":"Default to JSON"}`),
			expectedHeaders: http.Header{"Content-Type": []string{"application/json; charset=utf-8"}},
		},
		{
			name:         "text/html",
			verb:         stringVerb,
			headers:      map[string][]string{"Content-Type": {"text/html"}},
			body:         []byte(`"<html><body>Hello, World!</body></html>"`),
			expectedBody: []byte("<html><body>Hello, World!</body></html>"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, headers, err := ResponseForVerb(sch, tc.verb, HTTPResponse{Body: tc.body, Headers: tc.headers})
			assert.NoError(t, err)
			assert.Equal(t, tc.expectedBody, result)
			if tc.expectedHeaders != nil {
				assert.Equal(t, tc.expectedHeaders, headers)
			}
		})
	}
}

func TestValueForData(t *testing.T) {
	tests := []struct {
		typ    schema.Type
		data   []byte
		result any
	}{
		{&schema.String{}, []byte("test"), "test"},
		{&schema.Int{}, []byte("1234"), 1234},
		{&schema.Float{}, []byte("12.34"), 12.34},
		{&schema.Bool{}, []byte("true"), true},
		{&schema.Array{Element: &schema.String{}}, []byte(`["test1", "test2"]`), []any{"test1", "test2"}},
		{&schema.Map{Key: &schema.String{}, Value: &schema.String{}}, []byte(`{"key1": "value1", "key2": "value2"}`), obj{"key1": "value1", "key2": "value2"}},
		{&schema.DataRef{Module: "test", Name: "Test"}, []byte(`{"intValue": 10.0}`), obj{"intValue": 10.0}},
	}

	for _, test := range tests {
		result, err := valueForData(test.typ, test.data)
		assert.NoError(t, err)
		assert.Equal(t, test.result, result)
	}
}
