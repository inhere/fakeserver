package recorder

import (
	"context"
	"testing"
)

func TestRequestTraceContext(t *testing.T) {
	ctx, trace := WithRequestTrace(context.Background())
	routeIndex := 2
	caseIndex := 1

	SetRouteMatch(ctx, RequestTrace{
		RouteIndex:  &routeIndex,
		CaseIndex:   &caseIndex,
		RouteMode:   "cases",
		RouteSource: ".fakeserver/routes/users.json5",
		ProxyTarget: "https://example.test",
	})

	if trace.RouteIndex == nil || *trace.RouteIndex != routeIndex {
		t.Fatalf("RouteIndex=%v, want %d", trace.RouteIndex, routeIndex)
	}
	if trace.CaseIndex == nil || *trace.CaseIndex != caseIndex {
		t.Fatalf("CaseIndex=%v, want %d", trace.CaseIndex, caseIndex)
	}
	if trace.RouteMode != "cases" {
		t.Fatalf("RouteMode=%q, want cases", trace.RouteMode)
	}
	if trace.RouteSource != ".fakeserver/routes/users.json5" {
		t.Fatalf("RouteSource=%q", trace.RouteSource)
	}
	if trace.ProxyTarget != "https://example.test" {
		t.Fatalf("ProxyTarget=%q", trace.ProxyTarget)
	}

	got, ok := TraceFromContext(ctx)
	if !ok || got != trace {
		t.Fatalf("TraceFromContext ok=%v got=%p want=%p", ok, got, trace)
	}
}

func TestSetRouteMatch_IgnoresMissingTrace(t *testing.T) {
	SetRouteMatch(context.Background(), RequestTrace{RouteMode: "mock"})
}
