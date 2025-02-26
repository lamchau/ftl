package rpc

import (
	"context"
	"fmt"
	"net/http"

	"connectrpc.com/connect"
	"connectrpc.com/otelconnect"
	"golang.org/x/mod/semver"

	"github.com/TBD54566975/ftl"
	"github.com/TBD54566975/ftl/backend/schema"
	"github.com/TBD54566975/ftl/internal/log"
	"github.com/TBD54566975/ftl/internal/model"
	"github.com/TBD54566975/ftl/internal/rpc/headers"
)

type ftlDirectRoutingKey struct{}
type ftlVerbKey struct{}
type requestIDKey struct{}

// WithDirectRouting ensures any hops in Verb routing do not redirect.
//
// This is used so that eg. calls from Drives do not create recursive loops
// when calling back to the Agent.
func WithDirectRouting(ctx context.Context) context.Context {
	return context.WithValue(ctx, ftlDirectRoutingKey{}, "1")
}

// WithVerbs adds the module.verb chain from the current request to the context.
func WithVerbs(ctx context.Context, verbs []*schema.VerbRef) context.Context {
	return context.WithValue(ctx, ftlVerbKey{}, verbs)
}

// VerbFromContext returns the current module.verb of the current request.
func VerbFromContext(ctx context.Context) (*schema.VerbRef, bool) {
	value := ctx.Value(ftlVerbKey{})
	verbs, ok := value.([]*schema.VerbRef)
	if len(verbs) == 0 {
		return nil, false
	}
	return verbs[len(verbs)-1], ok
}

// VerbsFromContext returns the module.verb chain of the current request.
func VerbsFromContext(ctx context.Context) ([]*schema.VerbRef, bool) {
	value := ctx.Value(ftlVerbKey{})
	verbs, ok := value.([]*schema.VerbRef)
	return verbs, ok
}

// IsDirectRouted returns true if the incoming request should be directly
// routed and never redirected.
func IsDirectRouted(ctx context.Context) bool {
	return ctx.Value(ftlDirectRoutingKey{}) != nil
}

// RequestNameFromContext returns the request Key from the context, if any.
func RequestNameFromContext(ctx context.Context) (model.RequestName, bool, error) {
	value := ctx.Value(requestIDKey{})
	keyStr, ok := value.(string)
	if !ok {
		return "", false, nil
	}
	_, key, err := model.ParseRequestName(keyStr)
	if err != nil {
		return "", false, fmt.Errorf("%s: %w", "invalid request Key", err)
	}
	return key, true, nil
}

// WithRequestName adds the request Key to the context.
func WithRequestName(ctx context.Context, key model.RequestName) context.Context {
	return context.WithValue(ctx, requestIDKey{}, key.String())
}

func DefaultClientOptions(level log.Level) []connect.ClientOption {
	interceptors := []connect.Interceptor{MetadataInterceptor(log.Debug), otelInterceptor()}
	if ftl.Version != "dev" {
		interceptors = append(interceptors, versionInterceptor{})
	}
	return []connect.ClientOption{
		connect.WithGRPC(), // Use gRPC because some servers will not be using Connect.
		connect.WithInterceptors(interceptors...),
	}
}

func DefaultHandlerOptions() []connect.HandlerOption {
	interceptors := []connect.Interceptor{MetadataInterceptor(log.Debug), otelInterceptor()}
	if ftl.Version != "dev" {
		interceptors = append(interceptors, versionInterceptor{})
	}
	return []connect.HandlerOption{connect.WithInterceptors(interceptors...)}
}

func otelInterceptor() connect.Interceptor {
	otel, err := otelconnect.NewInterceptor(otelconnect.WithTrustRemote(), otelconnect.WithoutServerPeerAttributes())
	if err != nil {
		panic(err)
	}
	return otel
}

// MetadataInterceptor propagates FTL metadata through servers and clients.
//
// "errorLevel" is the level at which errors will be logged
func MetadataInterceptor(errorLevel log.Level) connect.Interceptor {
	return &metadataInterceptor{
		errorLevel: errorLevel,
	}
}

type metadataInterceptor struct {
	errorLevel log.Level
}

func (*metadataInterceptor) WrapStreamingClient(req connect.StreamingClientFunc) connect.StreamingClientFunc {
	return func(ctx context.Context, s connect.Spec) connect.StreamingClientConn {
		// TODO(aat): I can't figure out how to get the client headers here.
		logger := log.FromContext(ctx)
		logger.Tracef("%s (streaming client)", s.Procedure)
		return req(ctx, s)
	}
}

func (m *metadataInterceptor) WrapStreamingHandler(req connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, s connect.StreamingHandlerConn) error {
		logger := log.FromContext(ctx)
		logger.Tracef("%s (streaming handler)", s.Spec().Procedure)
		ctx, err := propagateHeaders(ctx, s.Spec().IsClient, s.RequestHeader())
		if err != nil {
			return err
		}
		err = req(ctx, s)
		if err != nil {
			if connect.CodeOf(err) == connect.CodeCanceled {
				return nil
			}
			logger.Logf(m.errorLevel, "Streaming RPC failed: %s: %s", err, s.Spec().Procedure)
			return err
		}
		return nil
	}
}

func (m *metadataInterceptor) WrapUnary(uf connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		logger := log.FromContext(ctx)
		logger.Tracef("%s (unary)", req.Spec().Procedure)
		ctx, err := propagateHeaders(ctx, req.Spec().IsClient, req.Header())
		if err != nil {
			return nil, err
		}
		resp, err := uf(ctx, req)
		if err != nil {
			logger.Logf(m.errorLevel, "Unary RPC failed: %s: %s", err, req.Spec().Procedure)
			return nil, err
		}
		return resp, nil
	}
}

type clientKey[Client Pingable] struct{}

// ContextWithClient returns a context with an RPC client attached.
func ContextWithClient[Client Pingable](ctx context.Context, client Client) context.Context {
	return context.WithValue(ctx, clientKey[Client]{}, client)
}

// ClientFromContext returns the given RPC client from the context, or panics.
func ClientFromContext[Client Pingable](ctx context.Context) Client {
	value := ctx.Value(clientKey[Client]{})
	if value == nil {
		panic("no RPC client in context")
	}
	return value.(Client) //nolint:forcetypeassert
}

func propagateHeaders(ctx context.Context, isClient bool, header http.Header) (context.Context, error) {
	if isClient {
		if IsDirectRouted(ctx) {
			headers.SetDirectRouted(header)
		}
		if verbs, ok := VerbsFromContext(ctx); ok {
			headers.SetCallers(header, verbs)
		}
		if key, ok, err := RequestNameFromContext(ctx); ok {
			if err != nil {
				return nil, err
			}
			if ok {
				headers.SetRequestName(header, key)
			}
		}
	} else {
		if headers.IsDirectRouted(header) {
			ctx = WithDirectRouting(ctx)
		}
		if verbs, err := headers.GetCallers(header); err != nil {
			return nil, err
		} else { //nolint:revive
			ctx = WithVerbs(ctx, verbs)
		}
		if key, ok, err := headers.GetRequestName(header); err != nil {
			return nil, err
		} else if ok {
			ctx = WithRequestName(ctx, key)
		}
	}
	return ctx, nil
}

// versionInterceptor reports a warning to the client if the client is older than the server.
type versionInterceptor struct{}

func (v versionInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (v versionInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return next
}

func (v versionInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, ar connect.AnyRequest) (connect.AnyResponse, error) {
		resp, err := next(ctx, ar)
		if err != nil {
			return nil, err
		}
		if ar.Spec().IsClient {
			if err := v.checkVersion(resp.Header()); err != nil {
				log.FromContext(ctx).Warnf("%s", err)
			}
		} else {
			resp.Header().Set("X-FTL-Version", ftl.Version)
		}
		return resp, nil
	}
}

func (v versionInterceptor) checkVersion(header http.Header) error {
	version := header.Get("X-FTL-Version")
	if semver.Compare(ftl.Version, version) < 0 {
		return fmt.Errorf("FTL client (%s) is older than server (%s), consider upgrading: https://github.com/TBD54566975/ftl/releases", ftl.Version, version)
	}
	return nil
}
