package ingress

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/TBD54566975/ftl/backend/controller/dal"
	"github.com/TBD54566975/ftl/backend/schema"
)

// BuildRequestBody extracts the HttpRequest body from an HTTP request.
func BuildRequestBody(route *dal.IngressRoute, r *http.Request, sch *schema.Schema) ([]byte, error) {
	verb := sch.ResolveVerbRef(&schema.VerbRef{Name: route.Verb, Module: route.Module})
	if verb == nil {
		return nil, fmt.Errorf("unknown verb %q", route.Verb)
	}

	request, ok := verb.Request.(*schema.DataRef)
	if !ok {
		return nil, fmt.Errorf("verb %s input must be a data structure", verb.Name)
	}

	var body []byte

	var requestMap map[string]any

	if metadata, ok := verb.GetMetadataIngress().Get(); ok && metadata.Type == "http" {
		pathParameters := map[string]any{}
		matchSegments(route.Path, r.URL.Path, func(segment, value string) {
			pathParameters[segment] = value
		})

		httpRequestBody, err := extractHTTPRequestBody(route, r, request, sch)
		if err != nil {
			return nil, err
		}

		// Since the query and header parameters are a `map[string][]string`
		// we need to convert them before they go through the `transformFromAliasedFields` call
		// otherwise they will fail the type check.
		queryMap := make(map[string]any)
		for key, values := range r.URL.Query() {
			valuesAny := make([]any, len(values))
			for i, v := range values {
				valuesAny[i] = v
			}
			queryMap[key] = valuesAny
		}

		headerMap := make(map[string]any)
		for key, values := range r.Header {
			valuesAny := make([]any, len(values))
			for i, v := range values {
				valuesAny[i] = v
			}
			headerMap[key] = valuesAny
		}

		requestMap = map[string]any{}
		requestMap["method"] = r.Method
		requestMap["path"] = r.URL.Path
		requestMap["pathParameters"] = pathParameters
		requestMap["query"] = queryMap
		requestMap["headers"] = headerMap
		requestMap["body"] = httpRequestBody
	} else {
		var err error
		requestMap, err = buildRequestMap(route, r, request, sch)
		if err != nil {
			return nil, err
		}
	}

	requestMap, err := transformFromAliasedFields(request, sch, requestMap)
	if err != nil {
		return nil, err
	}

	err = validateRequestMap(request, []string{request.String()}, requestMap, sch)
	if err != nil {
		return nil, err
	}

	body, err = json.Marshal(requestMap)
	if err != nil {
		return nil, err
	}

	return body, nil
}

func extractHTTPRequestBody(route *dal.IngressRoute, r *http.Request, dataRef *schema.DataRef, sch *schema.Schema) (any, error) {
	bodyField, err := getBodyField(dataRef, sch)
	if err != nil {
		return nil, err
	}

	if dataRef, ok := bodyField.Type.(*schema.DataRef); ok {
		return buildRequestMap(route, r, dataRef, sch)
	}

	bodyData, err := readRequestBody(r)
	if err != nil {
		return nil, err
	}

	return valueForData(bodyField.Type, bodyData)
}

func valueForData(typ schema.Type, data []byte) (any, error) {
	switch typ.(type) {
	case *schema.DataRef:
		var bodyMap map[string]any
		err := json.Unmarshal(data, &bodyMap)
		if err != nil {
			return nil, fmt.Errorf("HTTP request body is not valid JSON: %w", err)
		}
		return bodyMap, nil

	case *schema.Array:
		var rawData []json.RawMessage
		err := json.Unmarshal(data, &rawData)
		if err != nil {
			return nil, fmt.Errorf("HTTP request body is not a valid JSON array: %w", err)
		}

		arrayData := make([]any, len(rawData))
		for i, rawElement := range rawData {
			var parsedElement any
			err := json.Unmarshal(rawElement, &parsedElement)
			if err != nil {
				return nil, fmt.Errorf("failed to parse array element: %w", err)
			}
			arrayData[i] = parsedElement
		}

		return arrayData, nil

	case *schema.Map:
		var bodyMap map[string]any
		err := json.Unmarshal(data, &bodyMap)
		if err != nil {
			return nil, fmt.Errorf("HTTP request body is not valid JSON: %w", err)
		}
		return bodyMap, nil

	case *schema.Bytes:
		return data, nil

	case *schema.String:
		return string(data), nil

	case *schema.Int:
		intVal, err := strconv.ParseInt(string(data), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("failed to parse integer from request body: %w", err)
		}
		return intVal, nil

	case *schema.Float:
		floatVal, err := strconv.ParseFloat(string(data), 64)
		if err != nil {
			return nil, fmt.Errorf("failed to parse float from request body: %w", err)
		}
		return floatVal, nil

	case *schema.Bool:
		boolVal, err := strconv.ParseBool(string(data))
		if err != nil {
			return nil, fmt.Errorf("failed to parse boolean from request body: %w", err)
		}
		return boolVal, nil

	case *schema.Unit:
		return map[string]any{}, nil

	default:
		return nil, fmt.Errorf("unsupported data type %T", typ)
	}
}

func readRequestBody(r *http.Request) ([]byte, error) {
	defer r.Body.Close()
	bodyData, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading request body: %w", err)
	}
	return bodyData, nil
}

func buildRequestMap(route *dal.IngressRoute, r *http.Request, dataRef *schema.DataRef, sch *schema.Schema) (map[string]any, error) {
	requestMap := map[string]any{}
	matchSegments(route.Path, r.URL.Path, func(segment, value string) {
		requestMap[segment] = value
	})

	switch r.Method {
	case http.MethodPost, http.MethodPut:
		var bodyMap map[string]any
		err := json.NewDecoder(r.Body).Decode(&bodyMap)
		if err != nil {
			return nil, fmt.Errorf("HTTP request body is not valid JSON: %w", err)
		}

		// Merge bodyMap into params
		for k, v := range bodyMap {
			requestMap[k] = v
		}
	default:
		data, err := sch.ResolveDataRefMonomorphised(dataRef)
		if err != nil {
			return nil, err
		}

		queryMap, err := parseQueryParams(r.URL.Query(), data)
		if err != nil {
			return nil, fmt.Errorf("HTTP query params are not valid: %w", err)
		}

		for key, value := range queryMap {
			requestMap[key] = value
		}
	}

	return requestMap, nil
}

func validateRequestMap(dataRef *schema.DataRef, path path, request map[string]any, sch *schema.Schema) error {
	data, err := sch.ResolveDataRefMonomorphised(dataRef)
	if err != nil {
		return err
	}

	var errs []error
	for _, field := range data.Fields {
		fieldPath := append(path, "."+field.Name) //nolint:gocritic

		value, haveValue := request[field.Name]
		if !haveValue && !allowMissingField(field) {
			errs = append(errs, fmt.Errorf("%s is required", fieldPath))
			continue
		}

		if haveValue {
			err := validateValue(field.Type, fieldPath, value, sch)
			if err != nil {
				errs = append(errs, err)
			}
		}

	}

	return errors.Join(errs...)
}

// Fields of these types can be omitted from the JSON representation.
func allowMissingField(field *schema.Field) bool {
	switch field.Type.(type) {
	case *schema.Optional, *schema.Any, *schema.Array, *schema.Map, *schema.Bytes, *schema.Unit:
		return true

	case *schema.Bool, *schema.DataRef, *schema.Float, *schema.Int, *schema.String, *schema.Time:
	}
	return false
}

func parseQueryParams(values url.Values, data *schema.Data) (map[string]any, error) {
	if jsonStr, ok := values["@json"]; ok {
		if len(values) > 1 {
			return nil, fmt.Errorf("only '@json' parameter is allowed, but other parameters were found")
		}
		if len(jsonStr) > 1 {
			return nil, fmt.Errorf("'@json' parameter must be provided exactly once")
		}

		return decodeQueryJSON(jsonStr[0])
	}

	queryMap := make(map[string]any)
	for key, value := range values {
		if hasInvalidQueryChars(key) {
			return nil, fmt.Errorf("complex key %q is not supported, use '@json=' instead", key)
		}

		var field *schema.Field
		for _, f := range data.Fields {
			if (f.JSONAlias != "" && f.JSONAlias == key) || f.Name == key {
				field = f
			}
			for _, typeParam := range data.TypeParameters {
				if typeParam.Name == key {
					field = &schema.Field{
						Name: key,
						Type: &schema.DataRef{Pos: typeParam.Pos, Name: typeParam.Name},
					}
				}
			}
		}

		if field == nil {
			queryMap[key] = value
			continue
		}

		switch field.Type.(type) {
		case *schema.Bytes, *schema.Map, *schema.Optional, *schema.Time,
			*schema.Unit, *schema.DataRef, *schema.Any:

		case *schema.Int, *schema.Float, *schema.String, *schema.Bool:
			if len(value) > 1 {
				return nil, fmt.Errorf("multiple values for %q are not supported", key)
			}
			if hasInvalidQueryChars(value[0]) {
				return nil, fmt.Errorf("complex value %q is not supported, use '@json=' instead", value[0])
			}
			queryMap[key] = value[0]

		case *schema.Array:
			for _, v := range value {
				if hasInvalidQueryChars(v) {
					return nil, fmt.Errorf("complex value %q is not supported, use '@json=' instead", v)
				}
			}
			queryMap[key] = value

		default:
			panic(fmt.Sprintf("unsupported type %T for query parameter field %q", field.Type, key))
		}
	}

	return queryMap, nil
}

func decodeQueryJSON(query string) (map[string]any, error) {
	decodedJSONStr, err := url.QueryUnescape(query)
	if err != nil {
		return nil, fmt.Errorf("failed to decode '@json' query parameter: %w", err)
	}

	// Unmarshal the JSON string into a map
	var resultMap map[string]any
	err = json.Unmarshal([]byte(decodedJSONStr), &resultMap)
	if err != nil {
		return nil, fmt.Errorf("failed to parse '@json' query parameter: %w", err)
	}

	return resultMap, nil
}

func hasInvalidQueryChars(s string) bool {
	return strings.ContainsAny(s, "{}[]|\\^`")
}
