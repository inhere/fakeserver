package recorder

import "context"

type traceContextKey struct{}

// RequestTrace stores route-match metadata collected by downstream handlers.
type RequestTrace struct {
	RouteIndex     *int
	CaseIndex      *int
	RouteMode      string
	RouteSource    string
	ProxyTarget    string
	Scenario       string
	CaseName       string
	OverrideSource string
}

// WithRequestTrace attaches a mutable trace holder to ctx.
func WithRequestTrace(ctx context.Context) (context.Context, *RequestTrace) {
	trace := &RequestTrace{}
	return context.WithValue(ctx, traceContextKey{}, trace), trace
}

// TraceFromContext returns the mutable trace holder attached by WithRequestTrace.
func TraceFromContext(ctx context.Context) (*RequestTrace, bool) {
	trace, ok := ctx.Value(traceContextKey{}).(*RequestTrace)
	return trace, ok
}

// SetRouteMatch merges route-match metadata into the request trace when present.
func SetRouteMatch(ctx context.Context, update RequestTrace) {
	trace, ok := TraceFromContext(ctx)
	if !ok {
		return
	}
	if update.RouteIndex != nil {
		trace.RouteIndex = update.RouteIndex
	}
	if update.CaseIndex != nil {
		trace.CaseIndex = update.CaseIndex
	}
	if update.RouteMode != "" {
		trace.RouteMode = update.RouteMode
	}
	if update.RouteSource != "" {
		trace.RouteSource = update.RouteSource
	}
	if update.ProxyTarget != "" {
		trace.ProxyTarget = update.ProxyTarget
	}
	if update.Scenario != "" {
		trace.Scenario = update.Scenario
	}
	if update.CaseName != "" {
		trace.CaseName = update.CaseName
	}
	if update.OverrideSource != "" {
		trace.OverrideSource = update.OverrideSource
	}
}
